package collector

import (
	"context"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestBatchFlush_BySize(t *testing.T) {
	ch := make(chan int, 10)
	var mu sync.Mutex
	var flushed [][]int

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runBatcher(ctx, ch, 3, time.Minute, func(batch []int) {
			cp := make([]int, len(batch))
			copy(cp, batch)
			mu.Lock()
			flushed = append(flushed, cp)
			mu.Unlock()
		})
	}()

	ch <- 1
	ch <- 2
	ch <- 3 // should flush here
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(flushed) != 1 || len(flushed[0]) != 3 {
		t.Fatalf("expected 1 flush of size 3, got %v", flushed)
	}
	mu.Unlock()
	cancel()
	<-done
}

func TestBatchFlush_ByTicker(t *testing.T) {
	ch := make(chan int, 10)
	var mu sync.Mutex
	var flushed [][]int

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runBatcher(ctx, ch, 100, 50*time.Millisecond, func(batch []int) {
			cp := make([]int, len(batch))
			copy(cp, batch)
			mu.Lock()
			flushed = append(flushed, cp)
			mu.Unlock()
		})
	}()

	ch <- 1
	ch <- 2
	time.Sleep(200 * time.Millisecond) // wait for ticker

	mu.Lock()
	if len(flushed) == 0 {
		t.Fatal("expected at least one flush by ticker")
	}
	mu.Unlock()
	cancel()
	<-done
}

func TestPurgeWindow(t *testing.T) {
	retentionDays := 7
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	old := time.Now().AddDate(0, 0, -8)
	recent := time.Now().AddDate(0, 0, -1)
	if !old.Before(cutoff) {
		t.Error("old event should be before cutoff")
	}
	if recent.Before(cutoff) {
		t.Error("recent event should not be before cutoff")
	}
}

func TestKubeEventToStore(t *testing.T) {
	t.Run("normal event with both timestamps maps all fields correctly", func(t *testing.T) {
		first := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
		last := time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC)

		ke := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				UID:       types.UID("uid-123"),
				Namespace: "default",
			},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod",
				Name: "api-pod",
			},
			Reason:         "OOMKilled",
			Message:        "pod ran out of memory",
			Type:           "Warning",
			Count:          5,
			FirstTimestamp: metav1.NewTime(first),
			LastTimestamp:  metav1.NewTime(last),
		}

		e := kubeEventToStore(ke)

		if e == nil {
			t.Fatal("expected non-nil result")
		}
		if e.UID != "uid-123" {
			t.Errorf("UID: got %q, want %q", e.UID, "uid-123")
		}
		if e.Namespace != "default" {
			t.Errorf("Namespace: got %q, want %q", e.Namespace, "default")
		}
		if e.Kind != "Pod" {
			t.Errorf("Kind: got %q, want %q", e.Kind, "Pod")
		}
		if e.Name != "api-pod" {
			t.Errorf("Name: got %q, want %q", e.Name, "api-pod")
		}
		if e.Reason != "OOMKilled" {
			t.Errorf("Reason: got %q, want %q", e.Reason, "OOMKilled")
		}
		if e.Message != "pod ran out of memory" {
			t.Errorf("Message: got %q, want %q", e.Message, "pod ran out of memory")
		}
		if e.Type != "Warning" {
			t.Errorf("Type: got %q, want %q", e.Type, "Warning")
		}
		if e.Count != 5 {
			t.Errorf("Count: got %d, want 5", e.Count)
		}
		if !e.FirstTime.Equal(first) {
			t.Errorf("FirstTime: got %v, want %v", e.FirstTime, first)
		}
		if !e.LastTime.Equal(last) {
			t.Errorf("LastTime: got %v, want %v", e.LastTime, last)
		}
	})

	t.Run("nil input returns nil", func(t *testing.T) {
		e := kubeEventToStore(nil)
		if e != nil {
			t.Errorf("expected nil, got %+v", e)
		}
	})

	t.Run("only CreationTimestamp set uses it as FirstTime", func(t *testing.T) {
		creation := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)

		ke := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				UID:               types.UID("uid-456"),
				Namespace:         "kube-system",
				CreationTimestamp: metav1.NewTime(creation),
			},
			InvolvedObject: corev1.ObjectReference{Kind: "Node", Name: "node-1"},
			Reason:         "NodeNotReady",
			Message:        "node is not ready",
			Type:           "Warning",
			Count:          1,
			// FirstTimestamp and LastTimestamp left as zero value
		}

		e := kubeEventToStore(ke)

		if e == nil {
			t.Fatal("expected non-nil result")
		}
		if !e.FirstTime.Equal(creation) {
			t.Errorf("FirstTime: got %v, want %v (CreationTimestamp)", e.FirstTime, creation)
		}
		// LastTime should fall back to FirstTime when LastTimestamp is zero
		if !e.LastTime.Equal(creation) {
			t.Errorf("LastTime: got %v, want %v (same as FirstTime)", e.LastTime, creation)
		}
	})
}