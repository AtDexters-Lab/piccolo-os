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
    // sequential allocation with wrap-around; avoid collisions within this process
    // HostBind
    hb := a.nextInRange(a.nextHostBind, a.hostBindRange)
    startHB := hb
    for {
        if _, ok := a.usedHost[hb]; !ok {
            break
        }
        hb++
        hb = a.nextInRange(hb, a.hostBindRange)
        if hb == startHB {
            return 0, 0, fmt.Errorf("no available host-bind ports in range %d-%d", a.hostBindRange.Start, a.hostBindRange.End)
        }
    }
    a.usedHost[hb] = struct{}{}
    a.nextHostBind = hb + 1

    // Public
    pp := a.nextInRange(a.nextPublic, a.publicRange)
    startPP := pp
    for {
        if _, ok := a.usedPublic[pp]; !ok {
            break
        }
        pp++
        pp = a.nextInRange(pp, a.publicRange)
        if pp == startPP {
            return 0, 0, fmt.Errorf("no available public ports in range %d-%d", a.publicRange.Start, a.publicRange.End)
        }
    }
    a.usedPublic[pp] = struct{}{}
    a.nextPublic = pp + 1

    return hb, pp, nil
}

