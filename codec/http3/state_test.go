package http3

import (
	"errors"
	"reflect"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestStateManagerRejectsDuplicateSettings(t *testing.T) {
	manager := newHTTP3StateManager(t, StateManagerConfig{})
	if err := manager.readState(SettingsFrame{}); err != nil {
		t.Fatal(err)
	}
	if err := manager.readState(SettingsFrame{}); !errors.Is(err, ErrInvalidFrameOrder) {
		t.Fatalf("inbound err=%v, want ErrInvalidFrameOrder", err)
	}

	manager = newHTTP3StateManager(t, StateManagerConfig{})
	if err := manager.writeState(SettingsFrame{}); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeState(SettingsFrame{}); !errors.Is(err, ErrInvalidFrameOrder) {
		t.Fatalf("outbound err=%v, want ErrInvalidFrameOrder", err)
	}
}

func TestStateManagerAllowsServerPushAfterMaxPushID(t *testing.T) {
	manager := newHTTP3StateManager(t, StateManagerConfig{Server: true})
	if err := manager.writeState(PushPromiseBlock{PushID: 1}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("err=%v, want push rejected before MAX_PUSH_ID", err)
	}
	if err := manager.readState(MaxPushIDFrame{PushID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeState(PushPromiseBlock{PushID: 1}); err != nil {
		t.Fatal(err)
	}
	if got := manager.PushState(1); got != PushStatePromised {
		t.Fatalf("push state=%v, want promised", got)
	}
}

func TestStateManagerClientValidatesInboundPushPromiseLimit(t *testing.T) {
	manager := newHTTP3StateManager(t, StateManagerConfig{InitialMaxPushID: 2})
	if err := manager.readState(PushPromiseBlock{PushID: 2}); err != nil {
		t.Fatal(err)
	}
	if err := manager.readState(PushPromiseBlock{PushID: 3}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("err=%v, want invalid frame", err)
	}
}

func TestStateManagerRejectsMaxPushIDDecrease(t *testing.T) {
	manager := newHTTP3StateManager(t, StateManagerConfig{Server: true})
	if err := manager.readState(MaxPushIDFrame{PushID: 4}); err != nil {
		t.Fatal(err)
	}
	if err := manager.readState(MaxPushIDFrame{PushID: 3}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("err=%v, want invalid frame", err)
	}
}

func TestStateManagerRejectsGoAwayIncrease(t *testing.T) {
	manager := newHTTP3StateManager(t, StateManagerConfig{})
	if err := manager.readState(GoAwayFrame{ID: 8}); err != nil {
		t.Fatal(err)
	}
	if err := manager.readState(GoAwayFrame{ID: 9}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("inbound err=%v, want invalid frame", err)
	}

	manager = newHTTP3StateManager(t, StateManagerConfig{})
	if err := manager.writeState(GoAwayFrame{ID: 8}); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeState(GoAwayFrame{ID: 9}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("outbound err=%v, want invalid frame", err)
	}
}

func TestStateManagerCancelPushBlocksLaterPushPromise(t *testing.T) {
	client := newHTTP3StateManager(t, StateManagerConfig{InitialMaxPushID: 3})
	if err := client.writeState(CancelPushFrame{PushID: 2}); err != nil {
		t.Fatal(err)
	}
	if err := client.readState(PushPromiseBlock{PushID: 2}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("client err=%v, want canceled push rejected", err)
	}

	server := newHTTP3StateManager(t, StateManagerConfig{Server: true})
	if err := server.readState(MaxPushIDFrame{PushID: 3}); err != nil {
		t.Fatal(err)
	}
	if err := server.readState(CancelPushFrame{PushID: 2}); err != nil {
		t.Fatal(err)
	}
	if err := server.writeState(PushPromiseBlock{PushID: 2}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("server err=%v, want canceled push rejected", err)
	}
}

func TestStateManagerRejectsPriorityUpdatePushBeyondLimit(t *testing.T) {
	client := newHTTP3StateManager(t, StateManagerConfig{InitialMaxPushID: 1})
	if err := client.writeState(PriorityUpdateFrame{Type: FramePriorityUpdatePush, ElementID: 2}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("client err=%v, want invalid frame", err)
	}

	server := newHTTP3StateManager(t, StateManagerConfig{Server: true})
	if err := server.writeState(PriorityUpdateFrame{Type: FramePriorityUpdatePush, ElementID: 1}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("server err=%v, want invalid frame before MAX_PUSH_ID", err)
	}
	if err := server.readState(MaxPushIDFrame{PushID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := server.writeState(PriorityUpdateFrame{Type: FramePriorityUpdatePush, ElementID: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestStateManagerRejectsMaxPushIDFromServer(t *testing.T) {
	manager := newHTTP3StateManager(t, StateManagerConfig{})
	if err := manager.readState(MaxPushIDFrame{PushID: 1}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("err=%v, want invalid frame", err)
	}
}

func TestStateManagerPipelineInstallsSharedHandlerWhenConfigured(t *testing.T) {
	state := newHTTP3StateManager(t, StateManagerConfig{InitialMaxPushID: 1})
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ApplyRequestStreamPipeline(ch.Pipeline(), PipelineConfig{State: state}); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	wantNames := []string{
		HandlerNameHTTP3FrameDecoder,
		HandlerNameHTTP3HeaderDecoder,
		HandlerNameHTTP3FrameEncoder,
		HandlerNameHTTP3HeaderEncoder,
		HandlerNameHTTP3StateManager,
	}
	if got := ch.Pipeline().Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("pipeline names=%v, want %v", got, wantNames)
	}
	if err := ch.Write(PushPromiseBlock{PushID: 1}); err == nil {
		t.Fatal("client outbound PUSH_PROMISE must be rejected")
	}
}

func newHTTP3StateManager(t *testing.T, cfg StateManagerConfig) *StateManager {
	t.Helper()
	manager, err := NewStateManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
