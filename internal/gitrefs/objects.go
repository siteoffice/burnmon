package gitrefs

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Object types as git numbers them in a pack.
const (
	typeCommit = 1
	typeTree   = 2
	typeBlob   = 3
	typeTag    = 4
)

const (
	maxObjectSize = 64 << 20
	maxDeltaDepth = 512
)

// maxChainBytes caps the bytes allocated while reading one object, summed
// over its whole delta chain. A variable so tests can lower it.
var maxChainBytes uint64 = 64 << 20

// store reads objects of one git dir. It is used by one walk and closed after.
type store struct {
	gitDir   string
	loaded   bool
	packs    []*pack
	deadline time.Time // zero means none; set by the walk
	alloc    uint64    // bytes allocated for the object being read
}

func newStore(gitDir string) *store { return &store{gitDir: gitDir} }

func (s *store) close() {
	for _, p := range s.packs {
		p.close()
	}
	s.packs = nil
}

func (s *store) loadPacks() {
	if s.loaded {
		return
	}
	s.loaded = true
	idxs, _ := filepath.Glob(filepath.Join(s.gitDir, "objects", "pack", "*.idx"))
	for _, ip := range idxs {
		if p, err := openPack(ip); err == nil {
			s.packs = append(s.packs, p)
		}
	}
}

// charge counts n more allocated bytes against the current object read and
// checks the walk deadline. It is called before every allocation in a chain.
func (s *store) charge(n uint64) error {
	if !s.deadline.IsZero() && time.Now().After(s.deadline) {
		return errors.New("gitrefs: walk deadline passed inside an object")
	}
	if n > maxChainBytes || s.alloc > maxChainBytes-n {
		return errors.New("gitrefs: object chain allocates too much")
	}
	s.alloc += n
	return nil
}

// commitParents returns the parent names of a commit object.
func (s *store) commitParents(sha string) ([]string, error) {
	s.alloc = 0
	typ, body, err := s.readObject(sha, 0)
	if err != nil {
		return nil, err
	}
	if typ != typeCommit {
		return nil, errors.New("gitrefs: object is not a commit")
	}
	var parents []string
	for _, line := range strings.Split(string(body), "\n") {
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "parent ") {
			p := strings.TrimSpace(strings.TrimPrefix(line, "parent "))
			if !isHex40(p) {
				return nil, errors.New("gitrefs: bad parent line")
			}
			parents = append(parents, p)
		}
	}
	return parents, nil
}

// readObject returns the type and body of an object, loose first, then packs.
func (s *store) readObject(sha string, depth int) (int, []byte, error) {
	if depth > maxDeltaDepth {
		return 0, nil, errors.New("gitrefs: delta chain too deep")
	}
	if !isHex40(sha) {
		return 0, nil, errors.New("gitrefs: bad object name")
	}
	typ, body, err := readLoose(s.gitDir, sha)
	if err == nil {
		return typ, body, nil
	}
	if !os.IsNotExist(err) {
		return 0, nil, err
	}
	s.loadPacks()
	for _, p := range s.packs {
		if off, ok := p.find(sha); ok {
			return p.read(off, s, depth)
		}
	}
	return 0, nil, errors.New("gitrefs: object not found")
}

func readLoose(gitDir, sha string) (int, []byte, error) {
	f, err := os.Open(filepath.Join(gitDir, "objects", sha[:2], sha[2:]))
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()
	zr, err := zlib.NewReader(f)
	if err != nil {
		return 0, nil, err
	}
	defer zr.Close()
	data, err := io.ReadAll(io.LimitReader(zr, maxObjectSize+64))
	if err != nil {
		return 0, nil, err
	}
	nul := bytes.IndexByte(data, 0)
	if nul < 0 {
		return 0, nil, errors.New("gitrefs: loose object has no header")
	}
	name, sizeStr, ok := strings.Cut(string(data[:nul]), " ")
	if !ok {
		return 0, nil, errors.New("gitrefs: bad loose header")
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size < 0 || size != len(data)-nul-1 {
		return 0, nil, errors.New("gitrefs: loose object size mismatch")
	}
	var typ int
	switch name {
	case "commit":
		typ = typeCommit
	case "tree":
		typ = typeTree
	case "blob":
		typ = typeBlob
	case "tag":
		typ = typeTag
	default:
		return 0, nil, errors.New("gitrefs: unknown object type")
	}
	return typ, data[nul+1:], nil
}
