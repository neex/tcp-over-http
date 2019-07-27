package dns

import (
	"sync"
)

type Cache struct {
	m sync.RWMutex
}

func (c *Cache) GetResponse(request []byte) ([]byte, error) {
	c.m.RLock()
	defer c.m.RUnlock()

	//dns.ListenAndServe()
	return nil, nil
}
