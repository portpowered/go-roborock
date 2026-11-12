package mqtt

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const stallDeadline = 50 * time.Millisecond

// silentCall sends a request the broker never answers and returns the call's error.
func silentCall(ctx context.Context, session *Session) error {
	_, err := session.Call(ctx, "silent", nil)

	return err
}

// awaitRetired fails the test unless the session ended after its missed reply.
func awaitRetired(t *testing.T, session *Session) {
	t.Helper()

	select {
	case <-session.Done():
	case <-time.After(hubTestWait):
		t.Fatal("a missed reply deadline left the subscription open")
	}
}

func TestMissedReplyDeadlineRetiresTheLastDeviceAndRedials(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	stalled := hubOpen(t, hub, hubDeviceA)
	hubCall(t, stalled, hubDeviceA)

	ctx, cancel := context.WithTimeout(context.Background(), stallDeadline)
	defer cancel()

	err := silentCall(ctx, stalled)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("missed reply error=%v", err)
	}

	_ = assertErrorKind(t, err, roborockerrors.Timeout)

	awaitRetired(t, stalled)

	eventually(t, "the stalled connection to end", func() bool { return source.endedCount() == 1 })

	fresh := hubOpen(t, hub, hubDeviceA)
	hubCall(t, fresh, hubDeviceA)

	if dials := source.dialCount(); dials != 2 {
		t.Fatalf("dials=%d, want a fresh connection after the stall", dials)
	}
}

func TestMissedReplyDeadlineRetiresOnlyThatDevice(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	stalled := hubOpen(t, hub, hubDeviceA)
	other := hubOpen(t, hub, "device-b")

	ctx, cancel := context.WithTimeout(context.Background(), stallDeadline)
	defer cancel()

	err := silentCall(ctx, stalled)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("missed reply error=%v", err)
	}

	awaitRetired(t, stalled)

	eventually(t, "the stalled device's unsubscribe", func() bool {
		return slices.ContainsFunc(source.snapshot().unsubscribed, func(topic string) bool {
			return strings.HasSuffix(topic, "/"+hubDeviceA)
		})
	})

	hubCall(t, other, "device-b")

	if dials := source.dialCount(); dials != 1 {
		t.Fatalf("dials=%d, other devices keep the shared connection", dials)
	}
}

func TestCanceledCallKeepsTheSubscription(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	session := hubOpen(t, hub, hubDeviceA)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() { result <- silentCall(ctx, session) }()

	eventually(t, "the silent request to be sent", func() bool {
		session.mu.Lock()
		defer session.mu.Unlock()

		return len(session.pending) == 1
	})
	cancel()

	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}

	hubCall(t, session, hubDeviceA)

	if dials := source.dialCount(); dials != 1 {
		t.Fatalf("dials=%d, a canceled call keeps its connection", dials)
	}
}
