package auth

import (
	"fmt"
	"sync"
	"testing"
)

func TestClientOverridesConcurrentAccess(t *testing.T) {
	SetClientID("")
	SetClientSecret("")
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func(n int) {
			defer wg.Done()
			SetClientID(fmt.Sprintf("id-%d", n))
		}(i)
		go func(n int) {
			defer wg.Done()
			SetClientSecret(fmt.Sprintf("secret-%d", n))
		}(i)
		go func() {
			defer wg.Done()
			_ = ClientID()
		}()
		go func() {
			defer wg.Done()
			_ = ClientSecret()
		}()
	}
	wg.Wait()
}
