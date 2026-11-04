package aiimport

import (
	"testing"
	"time"
)

func TestLockUser_SerializesTheSameUserOnly(t *testing.T) {
	unlock := lockUser(424242)
	acquired := make(chan struct{})

	go func() {
		unlockSecond := lockUser(424242)
		close(acquired)
		unlockSecond()
	}()

	select {
	case <-acquired:
		t.Fatal("the second lock of the same user was acquired while the first one was held")
	case <-time.After(100 * time.Millisecond):
	}

	// other users are not blocked
	unlockOther := lockUser(424243)
	unlockOther()

	unlock()

	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("the second lock of the same user was not acquired after the first one was released")
	}
}
