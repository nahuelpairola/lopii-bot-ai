package messaging

import (
	"sync"
	"testing"
	"time"
)

// Dos updates del MISMO usuario no se solapan. Sin esto, dos altas con gap de
// categoría se pisan la fila de conversation_states (PK user_id) y la primera
// se pierde sin que nada avise.
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

// Y usuarios distintos NO se bloquean entre sí: el bot atiende a muchos, y
// serializar global lo dejaría atendiendo de a uno.
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

	// Los dos tienen que poder entrar ANTES de que ninguno suelte. Con un
	// candado global, el segundo no llegaría nunca y esto expira.
	for i := 0; i < 2; i++ {
		select {
		case <-arrancaron:
		case <-time.After(2 * time.Second):
			t.Fatal("un usuario quedó esperando el candado de otro")
		}
	}
	close(soltar)
}

// El unlock que se devuelve es el del mutex que se tomó: dos locks seguidos del
// mismo usuario no pueden soltar el del otro.
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
