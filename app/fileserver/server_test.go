package fileserver_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	iofs "io/fs"
	"log"
	"math"
	"math/rand"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"github.com/usnistgov/ndn-dpdk/app/fileserver"
	"github.com/usnistgov/ndn-dpdk/app/tg/tgtestenv"
	"github.com/usnistgov/ndn-dpdk/core/nnduration"
	"github.com/usnistgov/ndn-dpdk/iface/intface"
	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndn/an"
	"github.com/usnistgov/ndn-dpdk/ndn/endpoint"
	"github.com/usnistgov/ndn-dpdk/ndn/l3"
	"github.com/usnistgov/ndn-dpdk/ndn/rdr"
	"github.com/usnistgov/ndn-dpdk/ndn/rdr/ndn6file"
	"github.com/usnistgov/ndn-dpdk/ndn/segmented"
	"github.com/usnistgov/ndn-dpdk/ndn/tlv"
	"github.com/zyedidia/generic"
	"go.uber.org/atomic"
	"golang.org/x/exp/slices"
)

type FileServerFixture struct {
	face   *intface.IntFace
	p      *fileserver.Server
	fw     l3.Forwarder
	fwFace l3.FwFace

	timeout context.Context
}

func (f *FileServerFixture) RetrieveMetadata(name string) (m ndn6file.Metadata, e error) {
	return f.RetrieveMetadataOpts(name, endpoint.ConsumerOptions{
		Retx: endpoint.RetxOptions{Limit: 3},
	})
}

func (f *FileServerFixture) RetrieveMetadataOpts(name string, opts endpoint.ConsumerOptions) (m ndn6file.Metadata, e error) {
	opts.Fw = f.fw
	e = rdr.RetrieveMetadata(f.timeout, &m, ndn.ParseName(name), opts)
	return
}

func (FileServerFixture) LastSeg(t testing.TB, finalBlock ndn.NameComponent) (lastSeg tlv.NNI) {
	assert, _ := makeAR(t)
	lastSeg = math.MaxUint64
	if assert.True(finalBlock.Valid()) {
		assert.EqualValues(an.TtSegmentNameComponent, finalBlock.Type)
		assert.NoError(lastSeg.UnmarshalBinary(finalBlock.Value))
	}
	return
}

func (f *FileServerFixture) FetchPayload(m ndn6file.Metadata, lastSeg *tlv.NNI) (payload []byte, e error) {
	opts := segmented.FetchOptions{
		RetxLimit: 3,
		MaxCwnd:   256,
	}
	if lastSeg != nil {
		opts.SegmentEnd = 1 + uint64(*lastSeg)
	}
	return f.FetchPayloadOpts(m, opts)
}

func (f *FileServerFixture) FetchPayloadOpts(m ndn6file.Metadata, opts segmented.FetchOptions) (payload []byte, e error) {
	opts.Fw = f.fw
	fetcher := segmented.Fetch(m.Name, opts)
	return fetcher.Payload(f.timeout)
}

func (f *FileServerFixture) ListDirectory(m ndn6file.Metadata) (ls ndn6file.DirectoryListing, e error) {
	payload, e := f.FetchPayload(m, nil)
	if e != nil {
		return ls, e
	}
	e = ls.UnmarshalBinary(payload)
	return
}

func newFileServerFixture(t testing.TB, cfg fileserver.Config) (f *FileServerFixture) {
	f = &FileServerFixture{}
	_, require := makeAR(t)
	var e error

	f.face = intface.MustNew()
	t.Cleanup(func() { f.face.D.Close() })

	f.p, e = fileserver.New(f.face.D, cfg)
	require.NoError(e)
	t.Cleanup(func() { f.p.Close() })
	tgtestenv.Open(t, f.p)
	f.p.Launch()
	time.Sleep(time.Second)

	f.fw = l3.NewForwarder()
	f.fwFace, e = f.fw.AddFace(f.face.A)
	require.NoError(e)
	f.fwFace.AddRoute(ndn.ParseName("/"))

	var cancel context.CancelFunc
	f.timeout, cancel = context.WithTimeout(context.TODO(), 20*time.Second)
	t.Cleanup(cancel)
	return f
}

func TestServer(t *testing.T) {
	assert, require := makeAR(t)

	cfg := fileserver.Config{
		NThreads: 2,
		Mounts: []fileserver.Mount{
			{Prefix: ndn.ParseName("/usr/bin"), Path: "/usr/bin"},
			{Prefix: ndn.ParseName("/usr/local-bin"), Path: "/usr/local/bin"},
			{Prefix: ndn.ParseName("/usr/local-lib"), Path: "/usr/local/lib"},
		},
		SegmentLen:   3000,
		StatValidity: nnduration.Nanoseconds(100 * time.Millisecond),
		KeepFds:      4,
	}
	f := newFileServerFixture(t, cfg)

	for _, tt := range []struct {
		Filename      string
		Name          string
		SetSegmentEnd bool
	}{
		{"/usr/local/bin/dpdk-testpmd", "/usr/local-bin/dpdk-testpmd", true},
		{"/usr/bin/jq", "/usr/bin/jq", false},
	} {
		tt := tt
		t.Run(tt.Filename, func(t *testing.T) {
			t.Parallel()
			assert, require := makeAR(t)

			content, e := os.ReadFile(tt.Filename)
			require.NoError(e)
			digest := sha256.Sum256(content)

			m, e := f.RetrieveMetadata(tt.Name)
			require.NoError(e)

			lastSeg := f.LastSeg(t, m.FinalBlock)
			assert.EqualValues(cfg.SegmentLen, m.SegmentSize)
			assert.EqualValues(len(content), m.Size)
			assert.False(m.Mtime.IsZero())

			fetcherLastSeg := &lastSeg
			if !tt.SetSegmentEnd {
				fetcherLastSeg = nil
			}
			payload, e := f.FetchPayload(m, fetcherLastSeg)
			require.NoError(e)
			assert.Len(payload, len(content))
			assert.Equal(digest, sha256.Sum256(payload))
		})
	}

	for _, tt := range []struct {
		Dirname string
		Name    string
	}{
		{"/usr/bin", "/usr/bin"},
		{"/usr/local/bin", "/usr/local-bin/" + ndn6file.KeywordLs.String()},
	} {
		tt := tt
		t.Run(tt.Dirname, func(t *testing.T) {
			t.Parallel()
			assert, require := makeAR(t)

			dirEntries, e := os.ReadDir(tt.Dirname)
			require.NoError(e)
			dirEntryNames := map[string]bool{}
			for _, dirEntry := range dirEntries {
				filename, mode := dirEntry.Name(), dirEntry.Type()
				if mode&os.ModeSymlink != 0 {
					if info, e := os.Stat(path.Join(tt.Dirname, filename)); e == nil {
						mode = info.Mode()
					}
				}
				switch {
				case mode.IsRegular():
					dirEntryNames[filename] = false
				case mode.IsDir():
					dirEntryNames[filename] = true
				}
			}

			m, e := f.RetrieveMetadata(tt.Name)
			require.NoError(e)
			assert.False(m.FinalBlock.Valid())
			assert.False(m.Mtime.IsZero())

			ls, e := f.ListDirectory(m)
			require.NoError(e)

			for _, entry := range ls {
				filename := entry.Name()
				isDir, ok := dirEntryNames[filename]
				assert.True(ok, filename)
				assert.Equal(isDir, entry.IsDir(), filename)
				delete(dirEntryNames, filename)
			}
			assert.Empty(dirEntryNames)
		})
	}

	assert.NoFileExists("/usr/local/bin/ndndpdk-no-such-program")
	type notFoundTest struct {
		Name       string
		ExpectNack bool
	}
	notFoundTests := []notFoundTest{
		{"/usr/local-bin/ndndpdk-no-such-program", true},
		{"/no-such-mount/autoexec.bat", false},
		{"/usr/local-bin/bad/zero%00/filename", false},
		{"/usr/local-bin/bad/slash%2F/filename", false},
		{"/usr/local-bin/bad/.../filename", false},
		{"/usr/local-bin/bad/..../filename", false},
		{"/usr/local-bin/bad/...../filename", false},
		{"/usr/local-bin/bad/....../filename", true},
	}
	if dirEntries, e := os.ReadDir("/usr/local/lib"); assert.NoError(e) {
		for _, dirEntry := range dirEntries {
			if dirEntry.Type().IsRegular() {
				// cannot get file metadata with 32=ls component, but this opens file descriptor for testing keepFds limit
				notFoundTests = append(notFoundTests, notFoundTest{
					Name:       "/usr/local-lib/" + dirEntry.Name() + "/" + ndn6file.KeywordLs.String(),
					ExpectNack: true,
				})
			}
		}
		require.GreaterOrEqual(len(notFoundTests), cfg.NThreads*cfg.KeepFds)
	}

	for _, tt := range notFoundTests {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			assert, _ := makeAR(t)

			expectedErr := endpoint.ErrExpire
			if tt.ExpectNack {
				expectedErr = ndn.ErrContentType
			}

			_, e := f.RetrieveMetadata(tt.Name)
			assert.ErrorIs(e, expectedErr)
		})
	}

	t.Cleanup(func() {
		cnt := f.p.Counters()
		assert.Greater(cnt.ReqRead, uint64(0))
		assert.Greater(cnt.ReqLs, uint64(0))
		assert.Greater(cnt.ReqMetadata, uint64(0))
		assert.Greater(cnt.FdNew, uint64(0))
		assert.Greater(cnt.FdNotFound, uint64(0))
		assert.Greater(cnt.FdClose, uint64(0))
		assert.LessOrEqual(cnt.FdNew, cnt.FdClose+uint64(cfg.NThreads*cfg.KeepFds))
		assert.Greater(cnt.UringSubmit, uint64(0))
		assert.Greater(cnt.UringSubmitNonBlock, uint64(0))
		assert.Greater(cnt.SqeSubmit, uint64(0))
		cntJ, _ := json.Marshal(cnt)
		t.Log(string(cntJ))
	})
}

const (
	fuseInoRoot fuseops.InodeID = fuseops.RootInodeID + iota
	fuseInoDirA
	fuseInoDirB
	fuseInoFileX
	fuseInoFileY
	fuseInoFileZ
	fuseInoSymlinkP
	fuseInoSymlinkQ
	fuseInoSocket
	fuseInoPipe
	fuseInoBlockDev
	fuseInoCharDev
	fuseInoALo
	fuseInoAHi = fuseInoALo + 900
)

var fuseRootDir = []fuseutil.Dirent{
	{Offset: 1, Inode: fuseInoDirA, Name: "A", Type: fuseutil.DT_Directory},
	{Offset: 2, Inode: fuseInoDirB, Name: "B"},
	{Offset: 3, Inode: fuseInoFileX, Name: "X", Type: fuseutil.DT_File},
	{Offset: 4, Inode: fuseInoFileY, Name: "Y"},
	{Offset: 5, Inode: fuseInoFileZ, Name: "Z", Type: fuseutil.DT_File},
	{Offset: 6, Inode: fuseInoSymlinkP, Name: "P"},
	{Offset: 7, Inode: fuseInoSymlinkQ, Name: "Q", Type: fuseutil.DT_Link},
	{Offset: 8, Inode: fuseInoSocket, Name: "socket"},
	{Offset: 9, Inode: fuseInoPipe, Name: "pipe"},
	{Offset: 10, Inode: fuseInoBlockDev, Name: "block", Type: fuseutil.DT_Block},
	{Offset: 11, Inode: fuseInoCharDev, Name: "char"},
}

type fuseFS struct {
	fuseutil.NotImplementedFileSystem
	ctime    time.Time
	mtime    time.Time
	mtimeZ   atomic.Time
	payloadY []byte
	sizeZ    uint64
}

var _ fuseutil.FileSystem = (*fuseFS)(nil)

func (fs *fuseFS) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) error {
	switch op.Parent {
	case fuseInoRoot:
		i := slices.IndexFunc(fuseRootDir, func(de fuseutil.Dirent) bool {
			return de.Name == op.Name
		})
		if i < 0 {
			return fuse.ENOENT
		}
		op.Entry.Child = fuseRootDir[i].Inode
	case fuseInoDirA:
		i, e := strconv.ParseInt(op.Name, 16, 32)
		if e != nil || i < int64(fuseInoALo) || i > int64(fuseInoAHi) {
			return fuse.ENOENT
		}
		op.Entry.Child = fuseops.InodeID(i)
	default:
		return fuse.ENOENT
	}

	return fs.inoAttr(op.Entry.Child, &op.Entry.Attributes)
}

func (fs *fuseFS) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) error {
	return fs.inoAttr(op.Inode, &op.Attributes)
}

func (fs *fuseFS) inoAttr(ino fuseops.InodeID, attr *fuseops.InodeAttributes) error {
	attr.Nlink = 1
	attr.Mode = 0o777
	attr.Ctime = fs.ctime
	attr.Mtime = fs.mtime

	switch ino {
	case fuseInoRoot, fuseInoDirA, fuseInoDirB:
		attr.Mode |= iofs.ModeDir
	case fuseInoFileX:
		attr.Size = 0
	case fuseInoFileY:
		attr.Size = uint64(len(fs.payloadY))
	case fuseInoFileZ:
		attr.Size = fs.sizeZ
		attr.Mtime = fs.mtimeZ.Load()
	case fuseInoSymlinkP, fuseInoSymlinkQ:
		attr.Mode |= iofs.ModeSymlink
	case fuseInoSocket:
		attr.Mode |= iofs.ModeSocket
	case fuseInoPipe:
		attr.Mode |= iofs.ModeNamedPipe
	case fuseInoBlockDev:
		attr.Mode |= iofs.ModeDevice
	case fuseInoCharDev:
		attr.Mode |= iofs.ModeDevice | iofs.ModeCharDevice
	default:
		if ino < fuseInoALo || ino > fuseInoAHi {
			return fuse.ENOENT
		}
	}
	return nil
}

func (fs *fuseFS) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) (e error) {
	switch op.Inode {
	case fuseInoFileX:
		return nil
	case fuseInoFileY:
		reader := bytes.NewReader(fs.payloadY)
		op.BytesRead, e = reader.ReadAt(op.Dst, op.Offset)
		if e == io.EOF {
			return nil
		}
		return e
	case fuseInoFileZ:
		rand.Read(op.Dst)
		op.BytesRead = generic.Min(len(op.Dst), int(fs.sizeZ-uint64(op.Offset)))
		return nil
	default:
		return fuse.EIO
	}
}

func (fs *fuseFS) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) error {
	switch op.Inode {
	case fuseInoRoot:
		return fs.dirRoot(op)
	case fuseInoDirA:
		return fs.dirA(op)
	case fuseInoDirB:
		return nil
	default:
		return fuse.ENOTDIR
	}
}

func (fs *fuseFS) dirRoot(op *fuseops.ReadDirOp) error {
	if int(op.Offset) > len(fuseRootDir) {
		return fuse.EIO
	}

	for _, de := range fuseRootDir[op.Offset:] {
		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], de)
		if n == 0 {
			break
		}
		op.BytesRead += n
	}
	return nil
}

func (fs *fuseFS) dirA(op *fuseops.ReadDirOp) error {
	for ino := generic.Max(uint64(op.Offset), uint64(fuseInoALo)); ino <= uint64(fuseInoAHi); ino++ {
		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], fuseutil.Dirent{
			Offset: fuseops.DirOffset(1 + ino),
			Inode:  fuseops.InodeID(ino),
			Name:   fmt.Sprintf("%064x", ino),
			Type:   fuseutil.DT_File,
		})
		if n == 0 {
			break
		}
		op.BytesRead += n
	}
	return nil
}

func (fs *fuseFS) ReadSymlink(ctx context.Context, op *fuseops.ReadSymlinkOp) error {
	switch op.Inode {
	case fuseInoSymlinkP:
		op.Target = "B"
	case fuseInoSymlinkQ:
		op.Target = "socket"
	default:
		return fuse.ENOENT
	}
	return nil
}

func TestFuse(t *testing.T) {
	_, require := makeAR(t)
	dir := t.TempDir()

	var fs fuseFS
	fuseLogger := log.New(log.Writer(), "FUSE ", log.Ltime|log.Lmicroseconds)
	mount, e := fuse.Mount(dir, fuseutil.NewFileSystemServer(&fs), &fuse.MountConfig{
		FSName:                  "NDN-DPDK fileserver test suite",
		ReadOnly:                true,
		ErrorLogger:             fuseLogger,
		DebugLogger:             fuseLogger,
		DisableWritebackCaching: true,
		EnableNoOpenSupport:     true,
		EnableNoOpendirSupport:  true,
		EnableAsyncReads:        true,
	})
	require.NoError(e)
	t.Cleanup(func() {
		go fuse.Unmount(dir)
		mount.Join(context.Background())
	})

	cfg := fileserver.Config{
		Mounts: []fileserver.Mount{
			{Prefix: ndn.ParseName("/fs"), Path: dir},
		},
		SegmentLen:   1200,
		StatValidity: nnduration.Nanoseconds(100 * time.Millisecond),
		KeepFds:      4,
	}
	f := newFileServerFixture(t, cfg)

	fs.ctime = time.Unix(1637712000, 0)
	fs.mtime = time.Unix(1644624000, 0)
	fs.mtimeZ.Store(fs.mtime)
	fs.payloadY = make([]byte, cfg.SegmentLen*15+1)
	rand.Read(fs.payloadY)
	fs.sizeZ = uint64(cfg.SegmentLen * 80000)

	t.Run("root", func(t *testing.T) {
		t.Parallel()
		assert, require := makeAR(t)

		m, e := f.RetrieveMetadata("/fs")
		require.NoError(e)
		ls, e := f.ListDirectory(m)
		require.NoError(e)

		require.Len(ls, 6)
		assert.Equal("A", ls[0].Name())
		assert.True(ls[0].IsDir())
		assert.Equal("B", ls[1].Name())
		assert.True(ls[1].IsDir())
		assert.Equal("X", ls[2].Name())
		assert.False(ls[2].IsDir())
		assert.Equal("Y", ls[3].Name())
		assert.False(ls[3].IsDir())
		assert.Equal("Z", ls[4].Name())
		assert.False(ls[4].IsDir())
		assert.Equal("P", ls[5].Name())
		assert.True(ls[5].IsDir())
	})

	t.Run("A", func(t *testing.T) {
		t.Parallel()
		assert, require := makeAR(t)

		m, e := f.RetrieveMetadata("/fs/A")
		require.NoError(e)
		assert.False(m.IsFile())
		assert.True(m.IsDir())
		assert.Equal(fs.ctime, m.Ctime)
		assert.Equal(fs.mtime, m.Mtime)

		ls, e := f.ListDirectory(m)
		require.NoError(e)
		assert.Len(ls, int(fuseInoAHi-fuseInoALo+1))
	})

	for _, suffix := range []string{"B", "P"} {
		t.Run(suffix, func(t *testing.T) {
			t.Parallel()
			assert, require := makeAR(t)

			m, e := f.RetrieveMetadata("/fs/" + suffix)
			require.NoError(e)
			assert.False(m.IsFile())
			assert.True(m.IsDir())

			ls, e := f.ListDirectory(m)
			require.NoError(e)
			assert.Len(ls, 0)
		})
	}

	t.Run("X", func(t *testing.T) {
		t.Parallel()
		assert, require := makeAR(t)

		m, e := f.RetrieveMetadata("/fs/X")
		require.NoError(e)
		assert.EqualValues(0, m.Size)
		assert.EqualValues(0, f.LastSeg(t, m.FinalBlock))

		payload, e := f.FetchPayload(m, nil)
		require.NoError(e)
		assert.Len(payload, 0)
	})

	t.Run("Y", func(t *testing.T) {
		t.Parallel()
		assert, require := makeAR(t)

		m, e := f.RetrieveMetadata("/fs/Y")
		require.NoError(e)
		assert.True(m.IsFile())
		assert.False(m.IsDir())
		assert.EqualValues(len(fs.payloadY), m.Size)
		assert.EqualValues(15, f.LastSeg(t, m.FinalBlock))
		assert.Equal(fs.ctime, m.Ctime)
		assert.Equal(fs.mtime, m.Mtime)

		payload, e := f.FetchPayload(m, nil)
		require.NoError(e)
		assert.Equal(fs.payloadY, payload)
	})

	t.Run("Z", func(t *testing.T) {
		t.Parallel()
		assert, require := makeAR(t)

		m, e := f.RetrieveMetadata("/fs/Z")
		require.NoError(e)
		assert.EqualValues(fs.sizeZ, m.Size)
		assert.EqualValues(79999, f.LastSeg(t, m.FinalBlock))
		assert.Equal(fs.ctime, m.Ctime)
		assert.Equal(fs.mtime, m.Mtime)

		lastSeg := tlv.NNI(3)
		payload, e := f.FetchPayload(m, &lastSeg)
		if assert.NoError(e) {
			assert.Len(payload, cfg.SegmentLen*4)
		}

		mtimeZ := fs.mtime.Add(8 * time.Second)
		fs.mtimeZ.Store(mtimeZ)
		time.Sleep(2 * cfg.StatValidity.Duration())
		_, e = f.FetchPayloadOpts(m, segmented.FetchOptions{
			SegmentEnd: 1 + uint64(lastSeg),
			RetxLimit:  0,
		})
		assert.Error(e)

		m, e = f.RetrieveMetadata("/fs/Z")
		require.NoError(e)
		assert.Equal(mtimeZ, m.Mtime)
	})

	for _, suffix := range []string{"socket", "pipe", "block", "char", "nonexistent"} {
		t.Run(suffix, func(t *testing.T) {
			if suffix == "pipe" {
				// XXX "pipe" causes openat() to block
				t.SkipNow()
			}

			t.Parallel()
			assert, _ := makeAR(t)

			_, e := f.RetrieveMetadata("/fs/" + suffix)
			assert.ErrorIs(e, ndn.ErrContentType)
		})
	}
}
