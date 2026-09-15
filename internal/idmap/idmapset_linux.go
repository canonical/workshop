package idmap

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/lxd/shared"
)

var (
	// ErrHostIdIsSubId is returned when an attempt is made to add an idmap entry
	// that intersects with an existing entry's host IDs.
	ErrHostIdIsSubId = errors.New("host id is in the range of subids") //nolint:revive
)

type IdRange struct { //nolint:revive
	Isuid   bool
	Isgid   bool
	Startid int64
	Endid   int64
}

// Contains checks if the given id is within the range defined by Startid and Endid.
func (i *IdRange) Contains(id int64) bool {
	return id >= i.Startid && id <= i.Endid
}

// ToLxcString returns the idmap entry in a format suitable for lxc.idmap.
func (e *IdmapEntry) ToLxcString() []string {
	digits := fmt.Sprintf("%d %d %d", e.Nsid, e.Hostid, e.Maprange)

	if e.Isuid && e.Isgid {
		return []string{
			"u " + digits,
			"g " + digits,
		}
	}

	if e.Isuid {
		return []string{"u " + digits}
	}

	return []string{"g " + digits}
}

// isBetween returns true if x is in the range [low, high).
func isBetween(x, low, high int64) bool {
	return x >= low && x < high
}

// HostidsIntersect checks if the host IDs of two idmap entries intersect.
func (e *IdmapEntry) HostidsIntersect(i IdmapEntry) bool {
	if (e.Isuid && i.Isuid) || (e.Isgid && i.Isgid) {
		switch {
		case isBetween(e.Hostid, i.Hostid, i.Hostid+i.Maprange):
			return true
		case isBetween(i.Hostid, e.Hostid, e.Hostid+e.Maprange):
			return true
		case isBetween(e.Hostid+e.Maprange, i.Hostid, i.Hostid+i.Maprange):
			return true
		case isBetween(i.Hostid+i.Maprange, e.Hostid, e.Hostid+e.Maprange):
			return true
		}
	}

	return false
}

// Intersects checks if two idmap entries intersect.
func (e *IdmapEntry) Intersects(i IdmapEntry) bool {
	if (e.Isuid && i.Isuid) || (e.Isgid && i.Isgid) {
		switch {
		case isBetween(e.Hostid, i.Hostid, i.Hostid+i.Maprange-1):
			return true
		case isBetween(i.Hostid, e.Hostid, e.Hostid+e.Maprange-1):
			return true
		case isBetween(e.Hostid+e.Maprange-1, i.Hostid, i.Hostid+i.Maprange-1):
			return true
		case isBetween(i.Hostid+i.Maprange-1, e.Hostid, e.Hostid+e.Maprange-1):
			return true
		case isBetween(e.Nsid, i.Nsid, i.Nsid+i.Maprange-1):
			return true
		case isBetween(i.Nsid, e.Nsid, e.Nsid+e.Maprange-1):
			return true
		case isBetween(e.Nsid+e.Maprange-1, i.Nsid, i.Nsid+i.Maprange-1):
			return true
		case isBetween(i.Nsid+i.Maprange-1, e.Nsid, e.Nsid+e.Maprange-1):
			return true
		}
	}
	return false
}

// Usable returns whether or not the idmap entry is usable in the current user namespace.
func (e *IdmapEntry) Usable() error {
	kernelIdmap, err := CurrentIdmapSet()
	if err != nil {
		return err
	}

	kernelRanges, err := kernelIdmap.ValidRanges()
	if err != nil {
		return err
	}

	// Validate the uid map
	if e.Isuid {
		valid := false
		for _, kernelRange := range kernelRanges {
			if !kernelRange.Isuid {
				continue
			}

			if kernelRange.Contains(e.Hostid) && kernelRange.Contains(e.Hostid+e.Maprange-1) {
				valid = true
				break
			}
		}

		if !valid {
			return fmt.Errorf("the %q map cannot work in the current user namespace", e.ToLxcString())
		}
	}

	// Validate the gid map
	if e.Isgid {
		valid := false
		for _, kernelRange := range kernelRanges {
			if !kernelRange.Isgid {
				continue
			}

			if kernelRange.Contains(e.Hostid) && kernelRange.Contains(e.Hostid+e.Maprange-1) {
				valid = true
				break
			}
		}

		if !valid {
			return fmt.Errorf("the %q map cannot work in the current user namespace", e.ToLxcString())
		}
	}

	return nil
}

// Len returns the length of the IdmapSet.
func (m IdmapSet) Len() int {
	return len(m.Idmap)
}

// Less compares the elements with indexes i and j.
func (m IdmapSet) Less(i, j int) bool {
	if m.Idmap[i].Isuid != m.Idmap[j].Isuid {
		return m.Idmap[i].Isuid
	}

	if m.Idmap[i].Isgid != m.Idmap[j].Isgid {
		return m.Idmap[i].Isgid
	}

	return m.Idmap[i].Nsid < m.Idmap[j].Nsid
}

// Swap swaps the elements with indexes i and j.
func (m IdmapSet) Swap(i, j int) {
	m.Idmap[i], m.Idmap[j] = m.Idmap[j], m.Idmap[i]
}

// Usable checks if all entries in the IdmapSet are usable in the current user namespace.
func (m IdmapSet) Usable() error {
	for _, e := range m.Idmap {
		err := e.Usable()
		if err != nil {
			return err
		}
	}

	return nil
}

// ValidRanges returns a list of valid ID ranges from the IdmapSet.
func (m IdmapSet) ValidRanges() ([]*IdRange, error) {
	ranges := []*IdRange{}

	// Sort the map
	idmap := IdmapSet{}
	err := shared.DeepCopy(&m, &idmap)
	if err != nil {
		return nil, err
	}

	sort.Sort(idmap)

	for _, mapEntry := range idmap.Idmap {
		var entry *IdRange
		for _, idEntry := range ranges {
			if mapEntry.Isuid != idEntry.Isuid || mapEntry.Isgid != idEntry.Isgid {
				continue
			}

			if idEntry.Endid+1 == mapEntry.Nsid {
				entry = idEntry
				break
			}
		}

		if entry != nil {
			entry.Endid = entry.Endid + mapEntry.Maprange
			continue
		}

		ranges = append(ranges, &IdRange{
			Isuid:   mapEntry.Isuid,
			Isgid:   mapEntry.Isgid,
			Startid: mapEntry.Nsid,
			Endid:   mapEntry.Nsid + mapEntry.Maprange - 1,
		})
	}

	return ranges, nil
}

// AddSafe adds an entry to the idmap set, breaking apart any ranges that the
// new idmap intersects with in the process.
func (m *IdmapSet) AddSafe(i IdmapEntry) error {
	// doAddSafe() can't properly handle mappings that
	// both UID and GID, because in this case the "i" idmapping
	// will be inserted twice which may result to a further bugs and issues.
	// Simplest solution is to split a "both" mapping into two separate ones
	// one for UIDs and another one for GIDs.
	newUidIdmapEntry := i //nolint:revive
	newUidIdmapEntry.Isgid = false
	err := m.doAddSafe(newUidIdmapEntry)
	if err != nil {
		return err
	}

	newGidIdmapEntry := i
	newGidIdmapEntry.Isuid = false
	err = m.doAddSafe(newGidIdmapEntry)
	if err != nil {
		return err
	}

	return nil
}

func (m *IdmapSet) doAddSafe(i IdmapEntry) error {
	result := []IdmapEntry{}
	added := false

	if !i.Isuid && !i.Isgid {
		return nil
	}

	for _, e := range m.Idmap {
		if !e.Intersects(i) {
			result = append(result, e)
			continue
		}

		if e.HostidsIntersect(i) {
			return ErrHostIdIsSubId
		}

		added = true

		lower := IdmapEntry{
			Isuid:    e.Isuid,
			Isgid:    e.Isgid,
			Hostid:   e.Hostid,
			Nsid:     e.Nsid,
			Maprange: i.Nsid - e.Nsid,
		}

		upper := IdmapEntry{
			Isuid:    e.Isuid,
			Isgid:    e.Isgid,
			Hostid:   e.Hostid + lower.Maprange + i.Maprange,
			Nsid:     i.Nsid + i.Maprange,
			Maprange: e.Maprange - i.Maprange - lower.Maprange,
		}

		if lower.Maprange > 0 {
			result = append(result, lower)
		}

		result = append(result, i)
		if upper.Maprange > 0 {
			result = append(result, upper)
		}
	}

	if !added {
		result = append(result, i)
	}

	m.Idmap = result
	return nil
}

// getFromProc gets a uid or gid mapping from /proc/self/{g,u}id_map.
func getFromProc(fname string) ([][]int64, error) {
	entries := [][]int64{}

	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Validate format
		s := strings.Fields(scanner.Text())
		if len(s) < 3 {
			return nil, fmt.Errorf("unexpected values in %q: %q", fname, s)
		}

		// Get range start
		entryStart, err := strconv.ParseUint(s[0], 10, 32)
		if err != nil {
			continue
		}

		// Get range size
		entryHost, err := strconv.ParseUint(s[1], 10, 32)
		if err != nil {
			continue
		}

		// Get range size
		entrySize, err := strconv.ParseUint(s[2], 10, 32)
		if err != nil {
			continue
		}

		entries = append(entries, []int64{int64(entryStart), int64(entryHost), int64(entrySize)})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("namespace does not have any map set")
	}

	return entries, nil
}

func KernelDefaultMap() (*IdmapSet, error) {
	idmapset := new(IdmapSet)

	kernelMap, err := CurrentIdmapSet()
	if err != nil {
		// Hardcoded fallback map
		e := IdmapEntry{Isuid: true, Isgid: false, Nsid: 0, Hostid: 1000000, Maprange: 1000000000}
		idmapset.Idmap = append(idmapset.Idmap, e)

		e = IdmapEntry{Isuid: false, Isgid: true, Nsid: 0, Hostid: 1000000, Maprange: 1000000000}
		idmapset.Idmap = append(idmapset.Idmap, e)
		return idmapset, nil //nolint:nilerr
	}

	// Look for mapped ranges
	kernelRanges, err := kernelMap.ValidRanges()
	if err != nil {
		return nil, err
	}

	// Special case for when we have the full kernel range
	fullKernelRanges := []*IdRange{
		{true, false, int64(0), int64(4294967294)},
		{false, true, int64(0), int64(4294967294)}}

	if reflect.DeepEqual(kernelRanges, fullKernelRanges) {
		// Hardcoded fallback map
		e := IdmapEntry{Isuid: true, Isgid: false, Nsid: 0, Hostid: 1000000, Maprange: 1000000000}
		idmapset.Idmap = append(idmapset.Idmap, e)

		e = IdmapEntry{Isuid: false, Isgid: true, Nsid: 0, Hostid: 1000000, Maprange: 1000000000}
		idmapset.Idmap = append(idmapset.Idmap, e)
		return idmapset, nil
	}

	// Find a suitable uid range
	for _, entry := range kernelRanges {
		// We only care about uids right now
		if !entry.Isuid {
			continue
		}

		// We want a map that's separate from the system's own POSIX allocation
		if entry.Endid < 100000 {
			continue
		}

		// Don't use the first 100000 ids
		if entry.Startid < 100000 {
			entry.Startid = 100000
		}

		// Check if we have enough ids
		if entry.Endid-entry.Startid < 65536 {
			continue
		}

		// Add the map
		e := IdmapEntry{Isuid: true, Isgid: false, Nsid: 0, Hostid: entry.Startid, Maprange: entry.Endid - entry.Startid + 1}
		idmapset.Idmap = append(idmapset.Idmap, e)

		// NOTE: Remove once LXD can deal with multiple shadow maps
		break
	}

	// Find a suitable gid range
	for _, entry := range kernelRanges {
		// We only care about gids right now
		if !entry.Isgid {
			continue
		}

		// We want a map that's separate from the system's own POSIX allocation
		if entry.Endid < 100000 {
			continue
		}

		// Don't use the first 65536 ids
		if entry.Startid < 100000 {
			entry.Startid = 100000
		}

		// Check if we have enough ids
		if entry.Endid-entry.Startid < 65536 {
			continue
		}

		// Add the map
		e := IdmapEntry{Isuid: false, Isgid: true, Nsid: 0, Hostid: entry.Startid, Maprange: entry.Endid - entry.Startid + 1}
		idmapset.Idmap = append(idmapset.Idmap, e)

		// NOTE: Remove once LXD can deal with multiple shadow maps
		break
	}

	return idmapset, nil
}

// CurrentIdmapSet creates an idmap of the current allocation.
func CurrentIdmapSet() (*IdmapSet, error) {
	idmapset := new(IdmapSet)

	// Parse the uidmap
	entries, err := getFromProc("/proc/self/uid_map")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		// Fallback map
		e := IdmapEntry{Isuid: true, Nsid: 0, Hostid: 0, Maprange: 0}
		idmapset.Idmap = append(idmapset.Idmap, e)
	} else {
		for _, entry := range entries {
			e := IdmapEntry{Isuid: true, Nsid: entry[0], Hostid: entry[1], Maprange: entry[2]}
			idmapset.Idmap = append(idmapset.Idmap, e)
		}
	}

	// Parse the gidmap
	entries, err = getFromProc("/proc/self/gid_map")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		// Fallback map
		e := IdmapEntry{Isgid: true, Nsid: 0, Hostid: 0, Maprange: 0}
		idmapset.Idmap = append(idmapset.Idmap, e)
	} else {
		for _, entry := range entries {
			e := IdmapEntry{Isgid: true, Nsid: entry[0], Hostid: entry[1], Maprange: entry[2]}
			idmapset.Idmap = append(idmapset.Idmap, e)
		}
	}

	return idmapset, nil
}
