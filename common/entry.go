package common

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/l-donovan/qcp/protocol"
)

type ThinDirEntry struct {
	Name string
	Mode fs.FileMode
}

func permBits(bits uint32) string {
	out := ""

	if bits&0b100 > 0 {
		out += "r"
	} else {
		out += "-"
	}

	if bits&0b010 > 0 {
		out += "w"
	} else {
		out += "-"
	}

	if bits&0b001 > 0 {
		out += "x"
	} else {
		out += "-"
	}

	return out
}

func modeString(mode fs.FileMode) string {
	out := ""

	if mode.IsDir() {
		out += "d"
	} else {
		out += "-"
	}

	perm := uint32(mode.Perm())

	userBits := (perm >> 6) & 0b111
	groupBits := (perm >> 3) & 0b111
	otherBits := (perm >> 0) & 0b111

	out += fmt.Sprintf("%s%s%s", permBits(userBits), permBits(groupBits), permBits(otherBits))

	return out
}
func (t ThinDirEntry) Title() string       { return t.Name }
func (t ThinDirEntry) Description() string { return modeString(t.Mode) }
func (t ThinDirEntry) FilterValue() string {
	return t.Name
}

func DeserializeDirEntry(serializedEntry string) (*ThinDirEntry, error) {
	components := strings.Split(serializedEntry, string(protocol.GroupSeparator))

	if len(components) != 2 {
		return nil, fmt.Errorf("expected 2 file entry components but got %d instead", len(components))
	}

	mode, err := strconv.Atoi(components[1])

	if err != nil {
		return nil, err
	}

	dirEntry := ThinDirEntry{
		Name: components[0],
		Mode: fs.FileMode(mode),
	}

	return &dirEntry, nil
}
