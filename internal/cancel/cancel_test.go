package cancel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestWatcher is a Watcher without Redis behind it, so the announcement
// handling can be exercised on its own.
func newTestWatcher() *Watcher { return &Watcher{jobs: map[string][]*Job{}} }

// A worker that could not reach Redis still gets a usable handle, so the task
// code needs no branches of its own.
func TestNilWatcherHandsOutAJobThatIsNeverCancelled(t *testing.T) {
	var w *Watcher

	job := w.Guard("some-task")
	require.NotNil(t, job)
	assert.False(t, job.Requested())

	// Releasing it frees the context, but nothing ever reported a request.
	job.Release()
	assert.False(t, job.Requested())
}

func TestAnnouncementCancelsTheJob(t *testing.T) {
	w := newTestWatcher()
	job := w.Guard("task-a")

	w.deliver("task-a")

	assert.True(t, job.Requested())
	<-job.Context().Done()
}

func TestAnnouncementForAnotherJobIsIgnored(t *testing.T) {
	w := newTestWatcher()
	job := w.Guard("task-a")

	w.deliver("task-b")

	assert.False(t, job.Requested())
	assert.NoError(t, job.Context().Err())
}

// An uninterruptible job records the cancellation but keeps running: this is
// what protects a reprepro run from being killed halfway through.
func TestUninterruptibleJobRecordsButKeepsRunning(t *testing.T) {
	w := newTestWatcher()
	job := w.Guard("task-a")
	job.Uninterruptible()

	w.deliver("task-a")

	assert.True(t, job.Requested())
	assert.NoError(t, job.Context().Err())
}

func TestReleaseStopsWatchingTheJob(t *testing.T) {
	w := newTestWatcher()
	job := w.Guard("task-a")

	job.Release()

	w.mu.Lock()
	_, still := w.jobs["task-a"]
	w.mu.Unlock()
	assert.False(t, still, "a released job should leave nothing behind")
}

func TestMarkKey(t *testing.T) {
	assert.Equal(t, "irgsh:cancel:task-a", MarkKey("task-a"))
}
