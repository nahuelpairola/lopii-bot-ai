package messaging

import (
	"sync"
	"testing"
	"time"
)

func TestUserLocks_SameUserIsSerialized(t *testing.T) {
	var locks userLocks
	var mu sync.Mutex
	var simultaneos, maxSimultaneos int

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer locks.lock(7)()

			mu.Lock()
			simultaneos++
			if simultaneos > maxSimultaneos {
				maxSimultaneos = simultaneos
			}
			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()
			simultaneos--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if maxSimultaneos != 1 {
		t.Errorf("hubo %d updates del mismo usuario a la vez, want 1", maxSimultaneos)
	}
}

func TestUserLocks_DifferentUsersRunInParallel(t *testing.T) {
	var locks userLocks
	arrancaron := make(chan struct{}, 2)
	soltar := make(chan struct{})

	for _, uid := range []uint64{1, 2} {
		go func(uid uint64) {
			defer locks.lock(uid)()
			arrancaron <- struct{}{}
			<-soltar
		}(uid)
	}

	for i := 0; i < 2; i++ {
		select {
		case <-arrancaron:
		case <-time.After(2 * time.Second):
			t.Fatal("un usuario quedó esperando el candado de otro")
		}
	}
	close(soltar)
}

func TestUserLocks_UnlockReleasesItsOwnMutex(t *testing.T) {
	var locks userLocks
	soltar := locks.lock(3)
	soltar()

	listo := make(chan struct{})
	go func() {
		defer locks.lock(3)()
		close(listo)
	}()
	select {
	case <-listo:
	case <-time.After(2 * time.Second):
		t.Fatal("el candado quedó tomado después de soltarlo")
	}
}
