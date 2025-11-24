package fsutil

import (
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/pkg/sftp"

	"github.com/canonical/workshop/internal/revert"
)

const (
	serverMkdirMode = 0o755
	serverOpenMode  = 0o644
)

var (
	sftpStatusError = regexp.MustCompile(`^sftp: (.*) \([^()]*\)$`)
	fileExistsError = regexp.MustCompile(`^([a-z]*) .*: file exists$`)
)

func NewSftpFs(client *sftp.Client, umask os.FileMode) Fs {
	return Fs{&sftpFs{client, umask}}
}

type sftpFs struct {
	client *sftp.Client
	umask  os.FileMode
}

var _ FsBackend = (*sftpFs)(nil)

func (s *sftpFs) Mkdir(path string, perm os.FileMode) error {
	if perm&^s.umask == serverMkdirMode {
		return s.mkdir(path)
	}

	rev := revert.New()
	defer rev.Fail()

	if err := s.mkdir(path); err != nil {
		return err
	}
	rev.Add(func() {
		_ = s.Remove(path)
	})

	if err := s.Chmod(path, perm&^s.umask); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (s *sftpFs) MkdirChmodChown(path string, perm os.FileMode, uid, gid int) error {
	rev := revert.New()
	defer rev.Fail()

	if err := s.mkdir(path); err != nil {
		return err
	}
	rev.Add(func() {
		_ = s.Remove(path)
	})

	if perm != serverMkdirMode {
		if err := s.Chmod(path, perm); err != nil {
			return err
		}
	}

	if uid != 0 || gid != 0 {
		if err := s.Chown(path, uid, gid); err != nil {
			return err
		}
	}

	rev.Success()
	return nil
}

func (s *sftpFs) mkdir(path string) error {
	err := s.client.Mkdir(path)
	return maybePathError("mkdir", path, err)
}

func (s *sftpFs) Open(path string) (File, error) {
	return s.openFile(path, os.O_RDONLY)
}

func (s *sftpFs) OpenFile(path string, flag int, perm os.FileMode) (File, error) {
	if flag&os.O_CREATE == 0 || perm&^s.umask == serverOpenMode {
		return s.openFile(path, flag)
	}
	if flag&os.O_EXCL == 0 {
		file, err := s.openFile(path, flag)
		if err != nil {
			return file, err
		}

		// FIXME: if we created the file, we should remove it on error.
		// If we didn't create it, we shouldn't call Chmod().
		if err := file.Chmod(perm &^ s.umask); err != nil {
			file.Close()
			return nil, err
		}

		return file, nil
	}

	rev := revert.New()
	defer rev.Fail()

	file, err := s.openFile(path, flag)
	if err != nil {
		return file, err
	}
	rev.Add(func() {
		_ = file.Close()
		_ = s.Remove(path)
	})

	if err := file.Chmod(perm &^ s.umask); err != nil {
		return nil, err
	}

	rev.Success()
	return file, nil
}

func (s *sftpFs) openFile(path string, flag int) (File, error) {
	file, err := s.client.OpenFile(path, flag)
	return file, maybePathError("open", path, err)
}

func (s *sftpFs) Symlink(source, target string) error {
	err := s.client.Symlink(source, target)
	return maybeLinkError("symlink", source, target, err)
}

func (s *sftpFs) ReadDir(path string) ([]os.FileInfo, error) {
	infos, err := s.client.ReadDir(path)
	return infos, maybePathError("open", path, err)
}

func (s *sftpFs) Stat(path string) (os.FileInfo, error) {
	info, err := s.client.Stat(path)
	return info, maybePathError("stat", path, err)
}

func (s *sftpFs) Lstat(path string) (os.FileInfo, error) {
	info, err := s.client.Lstat(path)
	return info, maybePathError("lstat", path, err)
}

func (s *sftpFs) Readlink(path string) (string, error) {
	link, err := s.client.ReadLink(path)
	return link, maybePathError("readlink", path, err)
}

func (s *sftpFs) Rename(source, target string) error {
	err := s.client.PosixRename(source, target)
	return maybeLinkError("rename", source, target, err)
}

func (s *sftpFs) Chmod(path string, mode os.FileMode) error {
	err := s.client.Chmod(path, mode)
	return maybePathError("chmod", path, err)
}

func (s *sftpFs) Chown(path string, uid, gid int) error {
	err := s.client.Chown(path, uid, gid)
	return maybePathError("chown", path, err)
}

func (s *sftpFs) Chtimes(path string, atime time.Time, mtime time.Time) error {
	err := s.client.Chtimes(path, atime, mtime)
	return maybePathError("chtimes", path, err)
}

func (s *sftpFs) Remove(path string) error {
	err := s.client.Remove(path)
	// sftp already uses PathError here, but we wrap just in case.
	return maybePathError("remove", path, err)
}

func (s *sftpFs) RemoveAll(path string) error {
	// FIXME: ideally this should use unlinkat, similar to os.RemoveAll.
	if err := s.client.RemoveAll(path); !errors.Is(err, os.ErrNotExist) {
		return maybePathError("open", path, err)
	}
	return nil
}

func (s *sftpFs) Close() error {
	return s.client.Close()
}

func maybePathError(op, path string, err error) error {
	// sftp discards the full error message when it returns one of these.
	if err == io.EOF || err == os.ErrNotExist || err == os.ErrPermission {
		return &os.PathError{Op: op, Path: path, Err: err}
	}

	var statusErr *sftp.StatusError
	if !errors.As(err, &statusErr) {
		return err
	}

	match := sftpStatusError.FindStringSubmatch(statusErr.Error())
	if match == nil {
		return err
	}

	message, err1 := strconv.Unquote(match[1])
	if err1 != nil {
		return err
	}

	if match = fileExistsError.FindStringSubmatch(message); match != nil {
		return &os.PathError{Op: match[1], Path: path, Err: os.ErrExist}
	}

	return errors.New(message)
}

func maybeLinkError(op, source, target string, err error) error {
	// sftp discards the full error message when it returns one of these.
	if err == io.EOF || err == os.ErrNotExist || err == os.ErrPermission {
		return &os.LinkError{Op: op, Old: source, New: target, Err: err}
	}
	return err
}
