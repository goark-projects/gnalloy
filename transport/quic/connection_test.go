package quic

import (
	"errors"
	"net"
	"testing"

	"goark.dev/gnalloy/transport/udp"
)

func TestConnectionIDRouterCreatesInitialConnectionAndRoutesByDCID(t *testing.T) {
	router := NewConnectionIDRouter(2)
	packet := Packet{
		Type:          PacketInitial,
		Version:       Version1,
		DestinationID: MustConnectionID([]byte{1, 2, 3}),
		SourceID:      MustConnectionID([]byte{4, 5}),
	}
	remote := udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 4433}
	conn, created, err := router.Route(packet, remote)
	if err != nil {
		t.Fatal(err)
	}
	if !created || conn.State != ConnectionStateNew || router.Len() != 1 {
		t.Fatalf("created=%v conn=%+v len=%d", created, conn, router.Len())
	}
	again, created, err := router.Route(packet, remote)
	if err != nil {
		t.Fatal(err)
	}
	if created || again != conn {
		t.Fatalf("created=%v again=%p conn=%p", created, again, conn)
	}
	router.Remove(packet.DestinationID)
	if _, ok := router.Find(packet.DestinationID); ok {
		t.Fatal("connection should be removed")
	}
}

func TestConnectionIDRouterRejectsUnknownNonInitial(t *testing.T) {
	router := NewConnectionIDRouter(2)
	_, _, err := router.Route(Packet{
		Type:          PacketHandshake,
		Version:       Version1,
		DestinationID: MustConnectionID([]byte{1}),
		SourceID:      MustConnectionID([]byte{2}),
	}, udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 4433})
	if !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("err=%v, want %v", err, ErrConnectionNotFound)
	}
}

func TestPacketSpaceClassifiesQUICPackets(t *testing.T) {
	if PacketSpace(PacketInitial) != PacketNumberSpaceInitial {
		t.Fatal("initial space mismatch")
	}
	if PacketSpace(PacketHandshake) != PacketNumberSpaceHandshake {
		t.Fatal("handshake space mismatch")
	}
	if PacketSpace(PacketShort) != PacketNumberSpaceApplication || PacketSpace(PacketZeroRTT) != PacketNumberSpaceApplication {
		t.Fatal("application space mismatch")
	}
}

func TestFiveTupleKeyIsComparableAndStable(t *testing.T) {
	local := udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 4433}
	remote := udp.Address{IP: net.IPv4(127, 0, 0, 2), Port: 50000}
	key, err := MakeFiveTupleKey(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	sessions := map[FiveTupleKey]string{key: "ok"}
	if sessions[key] != "ok" {
		t.Fatal("five tuple key should be usable as map key")
	}
}

func TestVersionNegotiationPacketBuilder(t *testing.T) {
	dcid := MustConnectionID([]byte{1, 2})
	scid := MustConnectionID([]byte{3, 4})
	packet, err := AppendVersionNegotiation(nil, dcid, scid, []Version{Version1})
	if err != nil {
		t.Fatal(err)
	}
	if !IsVersionNegotiation(packet) {
		t.Fatalf("not version negotiation: %v", packet)
	}
	if packet[5] != byte(dcid.Len()) || packet[8] != byte(scid.Len()) {
		t.Fatalf("cid lengths encoded incorrectly: %v", packet)
	}
}
