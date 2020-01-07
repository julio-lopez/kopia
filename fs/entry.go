package fs

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"time"
)

// Entry represents a filesystem entry, which can be Directory, File, or Symlink
type Entry interface {
	os.FileInfo
	Owner() OwnerInfo
}

// OwnerInfo describes owner of a filesystem entry
type OwnerInfo struct {
	UserID  uint32
	GroupID uint32
}

// Entries is a list of entries sorted by name.
type Entries []Entry

// Reader allows reading from a file and retrieving its up-to-date file info.
type Reader interface {
	io.ReadCloser
	io.Seeker

	Entry() (Entry, error)
}

// File represents an entry that is a file.
type File interface {
	Entry
	Open(ctx context.Context) (Reader, error)
}

// Directory represents contents of a directory.
type Directory interface {
	Entry
	Child(ctx context.Context, name string) (Entry, error)
	Readdir(ctx context.Context) (Entries, error)
	Summary() *DirectorySummary
}

// ErrEntryNotFound is returned when an entry is not found.
var ErrEntryNotFound = errors.New("entry not found")

// ReadDirAndFindChild reads all entries from a directory and returns one by name.
// This is a convenience function that may be helpful in implementations of Directory.Child()
func ReadDirAndFindChild(ctx context.Context, d Directory, name string) (Entry, error) {
	children, err := d.Readdir(ctx)
	if err != nil {
		return nil, err
	}

	e := children.FindByName(name)
	if e == nil {
		return nil, ErrEntryNotFound
	}

	return e, nil
}

// DirectorySummary represents summary information about a directory.
type DirectorySummary struct {
	TotalFileSize    int64     `json:"size"`
	TotalFileCount   int64     `json:"files"`
	TotalDirCount    int64     `json:"dirs"`
	MaxModTime       time.Time `json:"maxTime"`
	IncompleteReason string    `json:"incomplete,omitempty"`
}

// Symlink represents a symbolic link entry.
type Symlink interface {
	Entry
	Readlink(ctx context.Context) (string, error)
}

// FindByName returns an entry with a given name, or nil if not found.
func (e Entries) FindByName(n string) Entry {
	i := sort.Search(
		len(e),
		func(i int) bool {
			return e[i].Name() >= n
		},
	)
	if i < len(e) && e[i].Name() == n {
		return e[i]
	}

	return nil
}

// Update returns a copy of Entries with the provided entry included, by either replacing
// existing entry with the same name or inserted in the appropriate place to maintain sorted order.
func (e Entries) Update(newEntry Entry) Entries {
	name := newEntry.Name()
	pos := sort.Search(len(e), func(i int) bool {
		return e[i].Name() >= name
	})

	insert := 1 // insert
	if pos < len(e) && e[pos].Name() == name {
		insert = 0 // replace
	}

	es := make(Entries, len(e)+insert)

	copy(es[0:pos], e[0:pos])
	es[pos] = newEntry
	copy(es[pos+1:], e[pos+1-insert:])

	return es
}

// Remove returns a copy of Entries with the provided entry removed, while maintaining sorted order.
func (e Entries) Remove(name string) Entries {
	pos := sort.Search(len(e), func(i int) bool {
		return e[i].Name() >= name
	})

	// not found
	if pos >= len(e) || e[pos].Name() != name {
		return e
	}

	es := make(Entries, len(e)-1)

	copy(es[0:pos], e[0:pos])
	copy(es[pos:], e[pos+1:])

	return es
}

// Sort sorts the entries by name.
func (e Entries) Sort() {
	sort.Slice(e, func(i, j int) bool {
		return e[i].Name() < e[j].Name()
	})
}
