package channel

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

type fakeReadyPoller struct {
	modified []transport.ReadyMask
}

func (p *fakeReadyPoller) Model() transport.PollerModel {
	return transport.PollerReadiness
}

func (p *fakeReadyPoller) Backend() transport.BackendKind {
	return transport.BackendMemory
}

func (p *fakeReadyPoller) Register(transport.FDRef, transport.ChannelID, transport.ReadyMask) error {
	return nil
}

func (p *fakeReadyPoller) Modify(_ transport.FDRef, interest transport.ReadyMask) error {
	p.modified = append(p.modified, interest)
	return nil
}

func (p *fakeReadyPoller) Deregister(transport.FDRef) error {
	return nil
}

func (p *fakeReadyPoller) Submit(transport.IORequest) error {
	return nil
}

func (p *fakeReadyPoller) Poll([]transport.PollEvent, int) (int, error) {
	return 0, nil
}

func (p *fakeReadyPoller) Wakeup() error {
	return nil
}

func (p *fakeReadyPoller) Close() error {
	return nil
}

type fakeCompletionPoller struct {
	submitted []transport.IORequest
	backend   transport.BackendKind
}

func (p *fakeCompletionPoller) Model() transport.PollerModel {
	return transport.PollerCompletion
}

func (p *fakeCompletionPoller) Backend() transport.BackendKind {
	if p.backend != 0 {
		return p.backend
	}
	return transport.BackendIOCP
}

func (p *fakeCompletionPoller) Register(transport.FDRef, transport.ChannelID, transport.ReadyMask) error {
	return nil
}

func (p *fakeCompletionPoller) Modify(transport.FDRef, transport.ReadyMask) error {
	return nil
}

func (p *fakeCompletionPoller) Deregister(transport.FDRef) error {
	return nil
}

func (p *fakeCompletionPoller) Submit(req transport.IORequest) error {
	if req.Buf != nil {
		req.Buf.Retain()
	}
	for _, buf := range req.Bufs {
		buf.Retain()
	}
	p.submitted = append(p.submitted, req)
	return nil
}

func (p *fakeCompletionPoller) Poll([]transport.PollEvent, int) (int, error) {
	return 0, nil
}

func (p *fakeCompletionPoller) Wakeup() error {
	return nil
}

func (p *fakeCompletionPoller) Close() error {
	return nil
}

type writeStep struct {
	n     int
	again bool
}

type partialWriteRW struct {
	steps  []writeStep
	writes []string
}

func (rw *partialWriteRW) Read(transport.FDRef, []byte) (int, bool, error) {
	return 0, true, nil
}

func (rw *partialWriteRW) Write(_ transport.FDRef, src []byte) (int, bool, error) {
	rw.writes = append(rw.writes, string(src))
	step := rw.steps[0]
	rw.steps = rw.steps[1:]
	return step.n, step.again, nil
}

func (rw *partialWriteRW) Close(transport.FDRef) error {
	return nil
}

type vectorWriteRW struct {
	n      int
	again  bool
	writes [][]string
	writev int
	scalar int
}

func (rw *vectorWriteRW) Read(transport.FDRef, []byte) (int, bool, error) {
	return 0, true, nil
}

func (rw *vectorWriteRW) Write(_ transport.FDRef, src []byte) (int, bool, error) {
	rw.scalar++
	rw.writes = append(rw.writes, []string{string(src)})
	if rw.n > 0 {
		return rw.n, rw.again, nil
	}
	return len(src), rw.again, nil
}

func (rw *vectorWriteRW) Writev(_ transport.FDRef, src [][]byte) (int, bool, error) {
	rw.writev++
	parts := make([]string, 0, len(src))
	total := 0
	for _, part := range src {
		parts = append(parts, string(part))
		total += len(part)
	}
	rw.writes = append(rw.writes, parts)
	if rw.n > 0 {
		return rw.n, rw.again, nil
	}
	return total, rw.again, nil
}

func (rw *vectorWriteRW) Close(transport.FDRef) error {
	return nil
}

type writabilityRecorder struct {
	changes int
}

func (r *writabilityRecorder) ChannelWritabilityChanged(*HandlerContext) {
	r.changes++
}

type inactiveRecorder struct {
	count int
}

func (r *inactiveRecorder) ChannelInactive(*HandlerContext) {
	r.count++
}

type releaseReadHandler struct {
	reads      int
	closeAfter bool
}

type fixedTestBuf struct {
	buffer.ByteBuf
	idx uint16
}

func (b fixedTestBuf) FixedBufferIndex() (uint16, bool) {
	return b.idx, true
}

func (h *releaseReadHandler) ChannelRead(ctx *HandlerContext, msg any) {
	h.reads++
	if buf, ok := msg.(buffer.ByteBuf); ok {
		buf.Release()
	}
	if h.closeAfter {
		_ = ctx.Pipeline().Close()
	}
}

func TestUnsafeOutboundPartialWrite(t *testing.T) {
	poller := &fakeReadyPoller{}
	rw := &partialWriteRW{steps: []writeStep{{n: 2, again: true}, {n: 2}}}
	ch, unsafeCh := NewUnsafeChannel(UnsafeConfig{
		ID:                 1,
		FD:                 transport.FDRef{FD: 1},
		Allocator:          buffer.NewHeapAllocator(),
		Poller:             poller,
		ReadWriter:         rw,
		WriteHighWatermark: 4,
		WriteLowWatermark:  1,
	})
	recorder := &writabilityRecorder{}
	if err := ch.Pipeline().AddLast("writable", recorder); err != nil {
		t.Fatal(err)
	}

	buf := buffer.NewHeapBuffer(4)
	if _, err := buf.WriteBytes([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if err := ch.WriteAndFlush(buf); err != nil {
		t.Fatal(err)
	}
	if ch.IsWritable() {
		t.Fatal("channel should be unwritable after crossing high watermark")
	}
	if len(poller.modified) != 1 || poller.modified[0] != transport.ReadyRead|transport.ReadyWrite {
		t.Fatalf("modified=%v", poller.modified)
	}

	unsafeCh.HandleEvent(transport.PollEvent{Model: transport.PollerReadiness, Ready: transport.ReadyWrite})
	if !ch.IsWritable() {
		t.Fatal("channel should become writable after draining below low watermark")
	}
	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}
	if len(rw.writes) != 2 || rw.writes[0] != "abcd" || rw.writes[1] != "cd" {
		t.Fatalf("writes=%v", rw.writes)
	}
	if len(poller.modified) != 2 || poller.modified[1] != transport.ReadyRead {
		t.Fatalf("modified=%v", poller.modified)
	}
	if recorder.changes != 2 {
		t.Fatalf("writability changes=%d, want 2", recorder.changes)
	}
}

func TestUnsafeReadinessUsesGatheringWrite(t *testing.T) {
	rw := &vectorWriteRW{}
	ch, _ := NewUnsafeChannel(UnsafeConfig{
		ID:         1,
		FD:         transport.FDRef{FD: 1},
		Allocator:  buffer.NewHeapAllocator(),
		Poller:     &fakeReadyPoller{},
		ReadWriter: rw,
	})

	a := buffer.NewHeapBuffer(4)
	_, _ = a.WriteBytes([]byte("ab"))
	b := buffer.NewHeapBuffer(4)
	_, _ = b.WriteBytes([]byte("cd"))
	if err := ch.Write(a); err != nil {
		t.Fatal(err)
	}
	if err := ch.WriteAndFlush(b); err != nil {
		t.Fatal(err)
	}
	if rw.writev != 1 || rw.scalar != 0 {
		t.Fatalf("writev=%d scalar=%d", rw.writev, rw.scalar)
	}
	if len(rw.writes) != 1 || len(rw.writes[0]) != 2 || rw.writes[0][0] != "ab" || rw.writes[0][1] != "cd" {
		t.Fatalf("writes=%v", rw.writes)
	}
	if a.RefCnt() != 0 || b.RefCnt() != 0 {
		t.Fatalf("refs=%d,%d", a.RefCnt(), b.RefCnt())
	}
}

func TestUnsafeCompletionSubmitsGatheringWrite(t *testing.T) {
	poller := &fakeCompletionPoller{}
	ch, unsafeCh := NewUnsafeChannel(UnsafeConfig{
		ID:        1,
		FD:        transport.FDRef{FD: 1},
		Allocator: buffer.NewHeapAllocator(),
		Poller:    poller,
	})

	a := buffer.NewHeapBuffer(4)
	_, _ = a.WriteBytes([]byte("ab"))
	b := buffer.NewHeapBuffer(4)
	_, _ = b.WriteBytes([]byte("cd"))
	if err := ch.Write(a); err != nil {
		t.Fatal(err)
	}
	if err := ch.WriteAndFlush(b); err != nil {
		t.Fatal(err)
	}
	if len(poller.submitted) != 1 || len(poller.submitted[0].Bufs) != 2 {
		t.Fatalf("submitted=%+v", poller.submitted)
	}
	if a.RefCnt() != 2 || b.RefCnt() != 2 {
		t.Fatalf("pending refs=%d,%d, want 2,2", a.RefCnt(), b.RefCnt())
	}
	unsafeCh.HandleEvent(transport.PollEvent{
		Model: transport.PollerCompletion,
		Op:    transport.OpWrite,
		FD:    transport.FDRef{FD: 1},
		Bufs:  poller.submitted[0].Bufs,
		N:     4,
	})
	if a.RefCnt() != 0 || b.RefCnt() != 0 {
		t.Fatalf("refs=%d,%d, want 0,0", a.RefCnt(), b.RefCnt())
	}
}

func TestUnsafeCloseFiresInactiveOnce(t *testing.T) {
	ch, _ := NewUnsafeChannel(UnsafeConfig{
		ID:         1,
		FD:         transport.FDRef{FD: 1},
		Allocator:  buffer.NewHeapAllocator(),
		Poller:     &fakeReadyPoller{},
		ReadWriter: &partialWriteRW{},
	})
	recorder := &inactiveRecorder{}
	if err := ch.Pipeline().AddLast("inactive", recorder); err != nil {
		t.Fatal(err)
	}

	if err := ch.Pipeline().Close(); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.count != 1 {
		t.Fatalf("inactive count=%d, want 1", recorder.count)
	}
}

func TestUnsafeCompletionReadKeepsPendingBufferAliveUntilEvent(t *testing.T) {
	poller := &fakeCompletionPoller{}
	ch, unsafeCh := NewUnsafeChannel(UnsafeConfig{
		ID:             1,
		FD:             transport.FDRef{FD: 1},
		Allocator:      buffer.NewHeapAllocator(),
		Poller:         poller,
		ReadBufferSize: 4,
	})
	reader := &releaseReadHandler{closeAfter: true}
	if err := ch.Pipeline().AddLast("reader", reader); err != nil {
		t.Fatal(err)
	}

	if err := unsafeCh.BeginRead(); err != nil {
		t.Fatal(err)
	}
	if len(poller.submitted) != 1 {
		t.Fatalf("submitted=%d, want 1", len(poller.submitted))
	}
	buf := poller.submitted[0].Buf
	if buf.RefCnt() != 1 {
		t.Fatalf("pending read ref=%d, want 1", buf.RefCnt())
	}
	if _, err := buf.WriteBytes([]byte("ping")); err != nil {
		t.Fatal(err)
	}

	unsafeCh.HandleEvent(transport.PollEvent{
		Model: transport.PollerCompletion,
		Op:    transport.OpRead,
		FD:    transport.FDRef{FD: 1},
		Buf:   buf,
		N:     4,
	})
	if reader.reads != 1 {
		t.Fatalf("reads=%d, want 1", reader.reads)
	}
	if buf.RefCnt() != 0 {
		t.Fatalf("read buf ref=%d, want 0", buf.RefCnt())
	}
}

func TestUnsafeCompletionWriteKeepsPendingBufferAliveUntilEvent(t *testing.T) {
	poller := &fakeCompletionPoller{}
	ch, unsafeCh := NewUnsafeChannel(UnsafeConfig{
		ID:        1,
		FD:        transport.FDRef{FD: 1},
		Allocator: buffer.NewHeapAllocator(),
		Poller:    poller,
	})

	buf := buffer.NewHeapBuffer(4)
	if _, err := buf.WriteBytes([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if err := ch.WriteAndFlush(buf); err != nil {
		t.Fatal(err)
	}
	if len(poller.submitted) != 1 {
		t.Fatalf("submitted=%d, want 1", len(poller.submitted))
	}
	if buf.RefCnt() != 2 {
		t.Fatalf("pending write ref=%d, want 2", buf.RefCnt())
	}

	unsafeCh.HandleEvent(transport.PollEvent{
		Model: transport.PollerCompletion,
		Op:    transport.OpWrite,
		FD:    transport.FDRef{FD: 1},
		Buf:   buf,
		N:     4,
	})
	if buf.RefCnt() != 0 {
		t.Fatalf("write buf ref=%d, want 0", buf.RefCnt())
	}
}

func TestUnsafeCompletionWriteUsesFixedBufferIndexForIOUring(t *testing.T) {
	poller := &fakeCompletionPoller{backend: transport.BackendIOUring}
	ch, _ := NewUnsafeChannel(UnsafeConfig{
		ID:           1,
		FD:           transport.FDRef{FD: 1},
		Allocator:    buffer.NewHeapAllocator(),
		Poller:       poller,
		FixedBuffers: true,
	})

	inner := buffer.NewHeapBuffer(4)
	if _, err := inner.WriteBytes([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if err := ch.WriteAndFlush(fixedTestBuf{ByteBuf: inner, idx: 7}); err != nil {
		t.Fatal(err)
	}
	if len(poller.submitted) != 1 {
		t.Fatalf("submitted=%d, want 1", len(poller.submitted))
	}
	req := poller.submitted[0]
	if !req.UseFixedBuffer || req.FixedBufferIndex != 7 {
		t.Fatalf("fixed request=%+v, want fixed buffer index 7", req)
	}
}

func TestUnsafeCompletionCloseFiresInactiveOnCloseCompletion(t *testing.T) {
	poller := &fakeCompletionPoller{}
	ch, unsafeCh := NewUnsafeChannel(UnsafeConfig{
		ID:        1,
		FD:        transport.FDRef{FD: 1},
		Allocator: buffer.NewHeapAllocator(),
		Poller:    poller,
	})
	unsafeCh.MarkRegistered()
	recorder := &inactiveRecorder{}
	if err := ch.Pipeline().AddLast("inactive", recorder); err != nil {
		t.Fatal(err)
	}

	if err := ch.Pipeline().Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.count != 0 {
		t.Fatalf("inactive count before completion=%d, want 0", recorder.count)
	}
	if len(poller.submitted) != 1 || poller.submitted[0].Op != transport.OpClose {
		t.Fatalf("submitted=%v, want close", poller.submitted)
	}

	unsafeCh.HandleEvent(transport.PollEvent{
		Model: transport.PollerCompletion,
		Op:    transport.OpClose,
		FD:    transport.FDRef{FD: 1},
	})
	if recorder.count != 1 {
		t.Fatalf("inactive count=%d, want 1", recorder.count)
	}
}

func TestUnsafeCompletionCloseBeforeRegisterFiresInactiveSynchronously(t *testing.T) {
	poller := &fakeCompletionPoller{}
	ch, _ := NewUnsafeChannel(UnsafeConfig{
		ID:        1,
		FD:        transport.FDRef{FD: 1},
		Allocator: buffer.NewHeapAllocator(),
		Poller:    poller,
	})
	recorder := &inactiveRecorder{}
	if err := ch.Pipeline().AddLast("inactive", recorder); err != nil {
		t.Fatal(err)
	}

	if err := ch.Pipeline().Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.count != 1 {
		t.Fatalf("inactive count=%d, want 1", recorder.count)
	}
	if len(poller.submitted) != 0 {
		t.Fatalf("submitted=%v, want no async close before register", poller.submitted)
	}
}
