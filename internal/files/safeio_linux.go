//go:build linux

package files

// safeio — TOCTOU symlink-race-resistant file mutations using openat2(RESOLVE_BENEATH).
//
// PROBLEM: jailJoinStrict() resolves a path STRING via EvalSymlinks and returns it.
// Mutations (os.Chmod/os.WriteFile/os.Rename/os.RemoveAll/os.Create) later operate
// on that string as root. A tenant can swap an intermediate directory with a symlink
// between the check and the operation, tricking root into mutating a file OUTSIDE the
// jail (LPE / local privilege escalation).
//
// SOLUTION: openat2(RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS) provides an atomic fd
// relative to home, following NO symlinks, unable to escape home. All operations
// happen through the fd/*at syscalls. "Resolve + operate" is a single kernel step;
// intermediate symlink swapping becomes impossible.
// AlmaLinux 10 / kernel 6.12 supports openat2.

import (
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type usrInfo struct{ UID, GID int }

func userLookup(name string) (usrInfo, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return usrInfo{}, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return usrInfo{UID: uid, GID: gid}, nil
}

const dirOpenFlags = unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK

// relClean reduces a user-supplied path to a home-relative, '..'-cleaned path.
// A "/" prefix is added and Clean is applied to lexically dissolve any '..' entries;
// the real enforcement is still handled by openat2's RESOLVE_BENEATH flag.
func relClean(userPath string) string {
	return strings.TrimPrefix(filepath.Clean("/"+userPath), "/")
}

// openHomeFd opens the home directory O_DIRECTORY. home (/home/c_<slug>) is created
// by root; /home is owned by root → the tenant cannot swap the home DIRECTORY ENTRY
// with a symlink, so opening home directly is safe. Sub-components are protected by openat2.
func openHomeFd(home string) (int, error) {
	return unix.Open(home, unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
}

// openAt2Beneath opens rel beneath home atomically, following NO symlinks, unable to
// escape home. Returns an *os.File (caller must Close).
func openAt2Beneath(home, rel string, flags int, mode uint32) (*os.File, error) {
	hf, err := openHomeFd(home)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(hf) }()
	p := relClean(rel)
	if p == "" {
		p = "."
	}
	how := &unix.OpenHow{
		Flags:   uint64(flags) | unix.O_CLOEXEC,
		Mode:    uint64(mode),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	}
	fd, err := unix.Openat2(hf, p, how)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(home, p)), nil
}

// isDirBeneath reports whether rel is a DIRECTORY under home (symlink-safe; errors on
// intermediate symlinks).
func isDirBeneath(home, rel string) (bool, error) {
	f, err := openAt2Beneath(home, rel, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	return st.IsDir(), nil
}

// safeParentFd opens the PARENT directory of rel under home symlink-free (raw fd)
// and returns the single-component leaf name. Caller must unix.Close(parentFd).
// Pinning the parent fd means intermediate components can no longer be swapped.
func safeParentFd(home, rel string) (parentFd int, leaf string, err error) {
	p := relClean(rel)
	parent := filepath.Dir(p) // "a/b" → "a", "f" → "."
	leaf = filepath.Base(p)
	f, err := openAt2Beneath(home, parent, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return -1, "", err
	}
	fd, err := unix.Dup(int(f.Fd()))
	_ = f.Close()
	if err != nil {
		return -1, "", err
	}
	return fd, leaf, nil
}

// tenantIDs returns the uid/gid of system user sk (c_<slug>).
func tenantIDs(sk string) (uid, gid int, ok bool) {
	uu, err := userLookup(sk)
	if err != nil {
		return 0, 0, false
	}
	return uu.UID, uu.GID, true
}

// withinHome reports whether p resides beneath the (symlink-resolved) home directory.
// A final safety belt for residual path-based operations like restorecon-by-path.
func withinHome(home, p string) bool {
	hr, err := filepath.EvalSymlinks(home)
	if err != nil {
		hr = home
	}
	pr, err := filepath.EvalSymlinks(p)
	if err != nil {
		pr = p
	}
	return pr == hr || strings.HasPrefix(pr, hr+string(filepath.Separator))
}

// restoreconFd takes the PINNED real path of fd (/proc/self/fd/N — kernel-resolved,
// immune to attacker symlinks) and runs restorecon if it is still under home.
// On Enforcing SELinux servers, files created by root receive the wrong context
// and nginx/PHP-FPM cannot read them without this step. The within-home check
// confines the relabel to the jail.
func restoreconFd(home string, f *os.File) {
	real, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(int(f.Fd())))
	if err != nil || !withinHome(home, real) {
		return
	}
	_, _ = exec.Command("restorecon", real).CombinedOutput()
}

// fchownRestoreFd chowns the fd to the tenant (symlink-safe: Fchown on the pinned
// inode) and corrects the SELinux context. The old path-based chown(abs, sk) used
// os.Chown which FOLLOWS symlinks — a risk of handing /etc/shadow to a tenant (LPE);
// Fchown works on the pinned inode instead.
func fchownRestoreFd(home string, f *os.File, sk string) {
	if uid, gid, ok := tenantIDs(sk); ok {
		_ = unix.Fchown(int(f.Fd()), uid, gid)
	}
	restoreconFd(home, f)
}

// ---- High-level symlink-safe mutations ----

// chmodBeneath is a symlink-safe chmod. The leaf is opened via openat2 (symlinks are
// REJECTED); Fchmod is applied. Intermediate swaps are blocked by the kernel.
func chmodBeneath(home, rel string, mode uint32) error {
	f, err := openAt2Beneath(home, rel, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return unix.Fchmod(int(f.Fd()), mode)
}

// writeBeneath is a symlink-safe file write (create/truncate). An existing file's
// permissions are preserved (open won't touch mode outside create); a new file gets
// createMode. The fd is then chowned to the tenant + restorecon'd.
func writeBeneath(home, rel string, data []byte, createMode uint32, sk string) error {
	f, err := openAt2Beneath(home, rel, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC, createMode)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		return err
	}
	fchownRestoreFd(home, f, sk)
	return nil
}

// createExclBeneath is a symlink-safe new-empty-file (O_EXCL). Returns unix.EEXIST
// if the file already exists.
func createExclBeneath(home, rel, sk string) error {
	f, err := openAt2Beneath(home, rel, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0644)
	if err != nil {
		return err
	}
	fchownRestoreFd(home, f, sk)
	return f.Close()
}

// copyStreamBeneath is a symlink-safe streaming write (upload). Copies from src to fd.
func copyStreamBeneath(home, rel string, src io.Reader, sk string) (int64, error) {
	f, err := openAt2Beneath(home, rel, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC, 0644)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	n, err := io.Copy(f, src)
	if err != nil {
		return n, err
	}
	fchownRestoreFd(home, f, sk)
	return n, nil
}

// mkdirAllBeneath is a symlink-safe `mkdir -p`. Each component is created via
// Mkdirat + O_NOFOLLOW openat; any symlink component is REJECTED by O_NOFOLLOW.
// Newly created directories are chowned to the tenant when sk != "".
func mkdirAllBeneath(home, rel, sk string) error {
	p := relClean(rel)
	hf, err := openHomeFd(home)
	if err != nil {
		return err
	}
	if p == "" || p == "." {
		_ = unix.Close(hf)
		return nil
	}
	dirfd := hf
	uid, gid, haveIDs := tenantIDs(sk)
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." {
			continue
		}
		created := false
		if err := unix.Mkdirat(dirfd, part, 0755); err == nil {
			created = true
		} else if err != unix.EEXIST {
			_ = unix.Close(dirfd)
			return err
		}
		nfd, err := unix.Openat(dirfd, part, dirOpenFlags, 0)
		_ = unix.Close(dirfd)
		if err != nil {
			return err
		}
		dirfd = nfd
		if created && haveIDs {
			_ = unix.Fchown(dirfd, uid, gid)
		}
	}
	_ = unix.Close(dirfd)
	return nil
}

// renameBeneath is a symlink-safe rename/move. Source and destination PARENTs are
// pinned via openat2; Renameat performs the move (rename does NOT follow the final
// component symlink — it moves the entry).
func renameBeneath(home, oldRel, newRel, sk string) error {
	if err := mkdirAllBeneath(home, filepath.Dir(relClean(newRel)), sk); err != nil {
		return err
	}
	of, oleaf, err := safeParentFd(home, oldRel)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(of) }()
	nf, nleaf, err := safeParentFd(home, newRel)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(nf) }()
	return unix.Renameat(of, oleaf, nf, nleaf)
}

// removeAllBeneath is a symlink-safe `rm -rf`. The parent is pinned, and the leaf is
// removed (unlink for files/symlinks; fd-recursive unlinkat for directories). Symlinks
// are never followed at any step.
func removeAllBeneath(home, rel string) error {
	pfd, leaf, err := safeParentFd(home, rel)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(pfd) }()
	return removeAt(pfd, leaf)
}

// removeAt recursively deletes name relative to dirfd (all operations relative to
// pinned fds, O_NOFOLLOW → symlinks never followed, jail escape impossible).
func removeAt(dirfd int, name string) error {
	if err := unix.Unlinkat(dirfd, name, 0); err == nil {
		return nil
	} else if err == unix.ENOENT {
		return nil
	} else if err != unix.EISDIR && err != unix.EPERM && err != unix.ENOTEMPTY {
		return err
	}
	cfd, err := unix.Openat(dirfd, name, dirOpenFlags, 0)
	if err != nil {
		return err
	}
	names, rerr := readdirnamesFd(cfd)
	if rerr != nil {
		_ = unix.Close(cfd)
		return rerr
	}
	for _, n := range names {
		if n == "." || n == ".." {
			continue
		}
		if e := removeAt(cfd, n); e != nil {
			_ = unix.Close(cfd)
			return e
		}
	}
	_ = unix.Close(cfd)
	return unix.Unlinkat(dirfd, name, unix.AT_REMOVEDIR)
}

// readdirnamesFd lists a raw dir fd by duplicating it and reading via os.File.
// The original fd remains owned by the caller.
func readdirnamesFd(dirfd int) ([]string, error) {
	dup, err := unix.Dup(dirfd)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(dup), "dir")
	names, err := f.Readdirnames(-1)
	_ = f.Close()
	return names, err
}

// copyTreeBeneath is a symlink-safe recursive copy. Source and destination PARENTs
// are pinned; files are opened O_NOFOLLOW (jail-external symlink CONTENT is never
// read → no information leak), symlinks are recreated as-is (readlink+symlinkat),
// directories are recursed.
func copyTreeBeneath(home, srcRel, dstRel, sk string) error {
	if err := mkdirAllBeneath(home, filepath.Dir(relClean(dstRel)), sk); err != nil {
		return err
	}
	sfd, sleaf, err := safeParentFd(home, srcRel)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(sfd) }()
	dfd, dleaf, err := safeParentFd(home, dstRel)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(dfd) }()
	uid, gid, haveIDs := tenantIDs(sk)
	return copyEntryAt(sfd, sleaf, dfd, dleaf, uid, gid, haveIDs)
}

func copyEntryAt(sdir int, sname string, ddir int, dname string, uid, gid int, haveIDs bool) error {
	var st unix.Stat_t
	if err := unix.Fstatat(sdir, sname, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		if err := unix.Mkdirat(ddir, dname, st.Mode&0o777); err != nil && err != unix.EEXIST {
			return err
		}
		ncd, err := unix.Openat(ddir, dname, dirOpenFlags, 0)
		if err != nil {
			return err
		}
		defer func() { _ = unix.Close(ncd) }()
		if haveIDs {
			_ = unix.Fchown(ncd, uid, gid)
		}
		nsd, err := unix.Openat(sdir, sname, dirOpenFlags, 0)
		if err != nil {
			return err
		}
		defer func() { _ = unix.Close(nsd) }()
		names, rerr := readdirnamesFd(nsd)
		if rerr != nil {
			return rerr
		}
		for _, n := range names {
			if n == "." || n == ".." {
				continue
			}
			if e := copyEntryAt(nsd, n, ncd, n, uid, gid, haveIDs); e != nil {
				return e
			}
		}
		return nil
	case unix.S_IFLNK:
		target, err := readlinkAt(sdir, sname)
		if err != nil {
			return err
		}
		_ = unix.Unlinkat(ddir, dname, 0)
		return unix.Symlinkat(target, ddir, dname)
	case unix.S_IFREG:
		return copyRegAt(sdir, sname, ddir, dname, st.Mode&0o777, uid, gid, haveIDs)
	default:
		return nil // skip special files
	}
}

func readlinkAt(dirfd int, name string) (string, error) {
	buf := make([]byte, 4096)
	n, err := unix.Readlinkat(dirfd, name, buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

func copyRegAt(sdir int, sname string, ddir int, dname string, perm uint32, uid, gid int, haveIDs bool) error {
	sf, err := unix.Openat(sdir, sname, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	in := os.NewFile(uintptr(sf), sname)
	defer func() { _ = in.Close() }()
	df, err := unix.Openat(ddir, dname, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_NOFOLLOW|unix.O_CLOEXEC, perm)
	if err != nil {
		return err
	}
	out := os.NewFile(uintptr(df), dname)
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if haveIDs {
		_ = unix.Fchown(df, uid, gid)
	}
	return nil
}
