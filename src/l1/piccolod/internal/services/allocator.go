package services

import (
	"fmt"
)

// PortAllocator allocates ephemeral ports within configured ranges (in-memory)
type PortAllocator struct {
	hostBindRange PortRange
	publicRange   PortRange
	nextHostBind  int
	nextPublic    int
	usedHost      map[int]struct{}
	usedPublic    map[int]struct{}
}

func NewPortAllocator(hostBind, public PortRange) *PortAllocator {
	return &PortAllocator{
		hostBindRange: hostBind,
		publicRange:   public,
		nextHostBind:  hostBind.Start,
		nextPublic:    public.Start,
		usedHost:      make(map[int]struct{}),
		usedPublic:    make(map[int]struct{}),
	}
}

func (a *PortAllocator) nextInRange(current int, r PortRange) int {
	if current > r.End {
		return r.Start
	}
	return current
}

func (a *PortAllocator) AllocatePair() (int, int, error) {
	hb, err := a.allocateHost()
	if err != nil {
		return 0, 0, err
	}
	pp, err := a.allocatePublic()
	if err != nil {
		a.freeHost(hb)
		return 0, 0, err
	}
	return hb, pp, nil
}

func (a *PortAllocator) allocateHost() (int, error) {
	hb := a.nextInRange(a.nextHostBind, a.hostBindRange)
	startHB := hb
	for {
		if _, ok := a.usedHost[hb]; !ok {
			a.usedHost[hb] = struct{}{}
			if hb >= a.nextHostBind {
				a.nextHostBind = hb + 1
			}
			return hb, nil
		}
		hb++
		hb = a.nextInRange(hb, a.hostBindRange)
		if hb == startHB {
			return 0, fmt.Errorf("no available host-bind ports in range %d-%d", a.hostBindRange.Start, a.hostBindRange.End)
		}
	}
}

func (a *PortAllocator) allocatePublic() (int, error) {
	pp := a.nextInRange(a.nextPublic, a.publicRange)
	startPP := pp
	for {
		if _, ok := a.usedPublic[pp]; !ok {
			a.usedPublic[pp] = struct{}{}
			if pp >= a.nextPublic {
				a.nextPublic = pp + 1
			}
			return pp, nil
		}
		pp++
		pp = a.nextInRange(pp, a.publicRange)
		if pp == startPP {
			return 0, fmt.Errorf("no available public ports in range %d-%d", a.publicRange.Start, a.publicRange.End)
		}
	}
}

// ReserveHost reserves an existing host-bind port so it won't be reused.
func (a *PortAllocator) ReserveHost(port int) error {
	if port < a.hostBindRange.Start || port > a.hostBindRange.End {
		return fmt.Errorf("host port %d outside allocator range %d-%d", port, a.hostBindRange.Start, a.hostBindRange.End)
	}
	if _, exists := a.usedHost[port]; exists {
		return fmt.Errorf("host port %d already reserved", port)
	}
	a.usedHost[port] = struct{}{}
	if port >= a.nextHostBind {
		a.nextHostBind = port + 1
	}
	return nil
}

// AllocatePublic allocates only a public proxy port.
func (a *PortAllocator) AllocatePublic() (int, error) {
	return a.allocatePublic()
}

func (a *PortAllocator) freeHost(port int) {
	delete(a.usedHost, port)
	if port < a.nextHostBind {
		a.nextHostBind = port
	}
}

func (a *PortAllocator) freePublic(port int) {
	delete(a.usedPublic, port)
	if port < a.nextPublic {
		a.nextPublic = port
	}
}

func (a *PortAllocator) Release(host, public int) {
	if host > 0 {
		a.freeHost(host)
	}
	if public > 0 {
		a.freePublic(public)
	}
}

func (a *PortAllocator) ReleaseHost(port int) {
	if port > 0 {
		a.freeHost(port)
	}
}

func (a *PortAllocator) ReleasePublic(port int) {
	if port > 0 {
		a.freePublic(port)
	}
}
