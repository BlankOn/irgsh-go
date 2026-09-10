package repository

import (
	"context"
	"log"

	"github.com/blankon/irgsh-go/internal/cancel"
)

// CancelSignal adapts the Redis backed cancel.Requester to the
// usecase.CancelSignal port.
type CancelSignal struct {
	requester *cancel.Requester
}

func NewCancelSignal(redisURL string) (*CancelSignal, error) {
	requester, err := cancel.NewRequester(redisURL)
	if err != nil {
		return nil, err
	}
	return &CancelSignal{requester: requester}, nil
}

func (c *CancelSignal) Close() error { return c.requester.Close() }

func (c *CancelSignal) Request(taskUUID string) error {
	return c.requester.Request(context.Background(), taskUUID)
}

// IsRequested reports whether the job carries a cancellation mark. A Redis
// error is reported as "not cancelled": it is read on every status query, and
// a job that is running must not start looking cancelled because Redis
// hiccuped.
func (c *CancelSignal) IsRequested(taskUUID string) bool {
	requested, err := c.requester.IsRequested(context.Background(), taskUUID)
	if err != nil {
		log.Printf("Failed to read the cancellation mark of %s: %v\n", taskUUID, err)
		return false
	}
	return requested
}
