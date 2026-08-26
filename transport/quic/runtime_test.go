package quic

import (
	"bytes"
	"errors"
	"net"
	"testing"

	"goark.dev/gnalloy/transport/udp"
)

func TestACKTrackerBuildsSparseACKFrame(t *testing.T) {
	tracker := NewACKTracker()
	for _, pn := range []uint64{1, 2, 3, 7, 8, 10} {
		if !tracker.Receive(PacketNumberSpaceApplication, pn) {
			t.Fatalf("packet %d should be new", pn)
		}
	}
	if tracker.Receive(PacketNumberSpaceApplication, 8) {
		t.Fatal("duplicate packet should not be accepted as new")
	}
	frame, ok := tracker.ACKFrame(PacketNumberSpaceApplication, 0)
	if !ok {
		t.Fatal("ack frame should be available")
	}
	if frame.LargestAcked != 10 || frame.FirstAckRange != 0 || len(frame.AdditionalRanges) != 2 {
		t.Fatalf("ack frame=%+v, want largest=10 and two additional ranges", frame)
	}
	ranges, err := ACKFrameRanges(frame)
	if err != nil {
		t.Fatal(err)
	}
	want := []PacketNumberRange{{Smallest: 1, Largest: 3}, {Smallest: 7, Largest: 8}, {Smallest: 10, Largest: 10}}
	if !rangesEqual(ranges, want) {
		t.Fatalf("ranges=%+v, want %+v", ranges, want)
	}
	if !ACKFrameContains(frame, 7) || ACKFrameContains(frame, 6) {
		t.Fatalf("ack contains mismatch for frame=%+v", frame)
	}
}

func TestLossRecoveryMarksAckedAndPacketThresholdLoss(t *testing.T) {
	recovery := NewLossRecovery(LossRecoveryConfig{PacketThreshold: 3})
	for pn := uint64(1); pn <= 6; pn++ {
		recovery.OnPacketSent(SentPacket{
			Space:        PacketNumberSpaceApplication,
			Number:       pn,
			Bytes:        1200,
			AckEliciting: true,
		})
	}
	acked, lost, err := recovery.OnACK(PacketNumberSpaceApplication, ACKFrame{LargestAcked: 6, FirstAckRange: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(acked) != 1 || acked[0].Number != 6 {
		t.Fatalf("acked=%+v, want packet 6", acked)
	}
	if len(lost) != 3 {
		t.Fatalf("lost=%+v, want packets 1..3 lost", lost)
	}
	if recovery.InFlight(PacketNumberSpaceApplication) != 2 {
		t.Fatalf("inflight=%d, want 2", recovery.InFlight(PacketNumberSpaceApplication))
	}
}

func TestCongestionControllerTracksWindowAndLoss(t *testing.T) {
	controller, err := NewCongestionController(CongestionConfig{
		MaxDatagramSize: 100,
		InitialWindow:   1000,
		MinimumWindow:   200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.OnPacketSent(900); err != nil {
		t.Fatal(err)
	}
	if err := controller.OnPacketSent(200); !errors.Is(err, ErrCongestionLimited) {
		t.Fatalf("err=%v, want %v", err, ErrCongestionLimited)
	}
	controller.OnPacketAcked(300)
	if controller.InFlight() != 600 || controller.Window() != 1300 {
		t.Fatalf("inflight=%d window=%d, want 600/1300", controller.InFlight(), controller.Window())
	}
	controller.OnPacketLost(600)
	if controller.InFlight() != 0 || controller.Window() != 650 {
		t.Fatalf("inflight=%d window=%d, want 0/650", controller.InFlight(), controller.Window())
	}
}

func TestStreamManagerEnforcesFlowControlAndFinState(t *testing.T) {
	manager := NewStreamManager(4, 4)
	if _, err := manager.ReserveSend(1, 4, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReserveSend(1, 1, false); !errors.Is(err, ErrFlowControl) {
		t.Fatalf("err=%v, want %v", err, ErrFlowControl)
	}
	received := quicTestBuf("1234")
	defer received.Release()
	if _, err := manager.Receive(StreamFrame{StreamID: 1, Data: received, Fin: true}); err != nil {
		t.Fatal(err)
	}
	stream, ok := manager.Get(1)
	if !ok || stream.State != StreamStateHalfClosedRemote || !stream.FinReceived {
		t.Fatalf("stream=%+v ok=%v, want half-closed-remote", stream, ok)
	}
	if _, err := manager.ReserveSend(1, 0, true); err != nil {
		t.Fatal(err)
	}
	if stream.State != StreamStateClosed {
		t.Fatalf("state=%v, want closed", stream.State)
	}
}

func TestPathManagerValidatesBeforeMigration(t *testing.T) {
	active := udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 4433}
	next := udp.Address{IP: net.IPv4(127, 0, 0, 2), Port: 4433}
	manager := NewPathManager(active)
	manager.rand = bytes.NewReader([]byte("12345678"))

	challenge, err := manager.Challenge(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Migrate(next); !errors.Is(err, ErrInvalidPacket) {
		t.Fatalf("err=%v, want %v before validation", err, ErrInvalidPacket)
	}
	if !manager.Validate(next, PathResponseFrame{Data: challenge.Data}) {
		t.Fatal("path response should validate candidate path")
	}
	if err := manager.Migrate(next); err != nil {
		t.Fatal(err)
	}
	if manager.Active().String() != next.String() {
		t.Fatalf("active=%s, want %s", manager.Active(), next)
	}
}

func TestConnectionRuntimeAppliesACKToCongestion(t *testing.T) {
	conn := &Connection{Remote: udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 4433}}
	runtime, err := conn.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	if runtime != conn.runtime {
		t.Fatal("runtime should be cached on connection")
	}
	if err := runtime.Congestion.OnPacketSent(1200); err != nil {
		t.Fatal(err)
	}
	runtime.Loss.OnPacketSent(SentPacket{
		Space:        PacketNumberSpaceApplication,
		Number:       1,
		Bytes:        1200,
		AckEliciting: true,
	})
	if err := runtime.ApplyFrame(PacketNumberSpaceApplication, ACKFrame{LargestAcked: 1, FirstAckRange: 0}); err != nil {
		t.Fatal(err)
	}
	if runtime.Congestion.InFlight() != 0 {
		t.Fatalf("inflight=%d, want 0", runtime.Congestion.InFlight())
	}
}

func BenchmarkQUICRuntimeApplyACK(b *testing.B) {
	conn := &Connection{Remote: udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 4433}}
	runtime, err := NewRuntime(conn, RuntimeConfig{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		number := uint64(i + 1)
		if err := runtime.Congestion.OnPacketSent(1); err != nil {
			b.Fatal(err)
		}
		runtime.Loss.OnPacketSent(SentPacket{
			Space:        PacketNumberSpaceApplication,
			Number:       number,
			Bytes:        1,
			AckEliciting: true,
		})
		if err := runtime.ApplyFrame(PacketNumberSpaceApplication, ACKFrame{LargestAcked: number, FirstAckRange: 0}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQUICRuntimeReceiveStream(b *testing.B) {
	runtime := &Runtime{Streams: NewStreamManager(65535, ^uint64(0))}
	data := quicTestBuf("abcd")
	defer data.Release()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		frame := StreamFrame{StreamID: 1, Offset: uint64(i * 4), Data: data.Retain()}
		if err := runtime.ApplyFrame(PacketNumberSpaceApplication, frame); err != nil {
			frame.Release()
			b.Fatal(err)
		}
		frame.Release()
	}
}

func rangesEqual(a []PacketNumberRange, b []PacketNumberRange) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
