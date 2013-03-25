package openstack

import (
	"fmt"
	"io"
	"launchpad.net/goose/errors"
	"launchpad.net/goose/swift"
	"launchpad.net/juju-core/environs"
	"os"
	"sync"
	"time"
)

// storage implements environs.Storage on an OpenStack container.
type storage struct {
	sync.Mutex
	madeContainer bool
	containerName string
	containerACL  swift.ACL
	swift         *swift.Client
}

// makeContainer makes the environment's control container, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *storage) makeContainer(containerName string, containerACL swift.ACL) error {
	s.Lock()
	defer s.Unlock()
	if s.madeContainer {
		return nil
	}
	// try to make the container - CreateContainer will succeed if the container already exists.
	err := s.swift.CreateContainer(containerName, containerACL)
	if err == nil {
		s.madeContainer = true
	}
	return err
}

func (s *storage) Put(file string, r io.Reader, length int64) error {
	if err := s.makeContainer(s.containerName, s.containerACL); err != nil {
		return fmt.Errorf("cannot make Swift control container: %v", err)
	}
	err := s.swift.PutReader(s.containerName, file, r, length)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control container %q: %v", file, s.containerName, err)
	}
	return nil
}

func (s *storage) Get(file string) (r io.ReadCloser, err error) {
	for a := shortAttempt.Start(); a.Next(); {
		r, err = s.swift.GetReader(s.containerName, file)
		if errors.IsNotFound(err) {
			continue
		}
	}
	err, _ = maybeNotFound(err)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *storage) URL(name string) (string, error) {
	// 10 years should be good enough.
	expires := time.Now().AddDate(10, 0, 0)
	return s.swift.SignedURL(s.containerName, name, expires)
}

func (s *storage) Remove(file string) error {
	err := s.swift.DeleteObject(s.containerName, file)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if err, ok := maybeNotFound(err); !ok {
		return err
	}
	return nil
}

func (s *storage) List(prefix string) ([]string, error) {
	contents, err := s.swift.List(s.containerName, prefix, "", "", 0)
	if err != nil {
		// If the container is not found, it's not an error
		// because it's only created when the first
		// file is put.
		if err, ok := maybeNotFound(err); !ok {
			return nil, err
		}
		return nil, nil
	}
	var names []string
	for _, item := range contents {
		names = append(names, item.Name)
	}
	fmt.Fprintf(os.Stderr, "Looking in %#v found %#v", s.containerName, names)
	return names, nil
}

func (s *storage) deleteAll() error {
	names, err := s.List("")
	if err != nil {
		return err
	}
	// Remove all the objects in parallel so that we incur less round-trips.
	// If we're in danger of having hundreds of objects,
	// we'll want to change this to limit the number
	// of concurrent operations.
	if len(names) > 100 {
		return fmt.Errorf("Too many files to remove: %d", len(names))
	}
	var wg sync.WaitGroup
	wg.Add(len(names))
	errc := make(chan error, len(names))
	for _, name := range names {
		name := name
		go func() {
			if err := s.Remove(name); err != nil {
				errc <- err
			}
			wg.Done()
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return fmt.Errorf("cannot delete all provider state: %v", err)
	default:
	}

	s.Lock()
	defer s.Unlock()
	// Even DeleteContainer fails, it won't harm if we try again - the operation
	// might have succeeded even if we get an error.
	s.madeContainer = false
	err = s.swift.DeleteContainer(s.containerName)
	err, ok := maybeNotFound(err)
	if ok {
		return nil
	}
	return err
}

// maybeNotFound returns a environs.NotFoundError if the root cause of the specified error is due to a file or
// container not being found.
func maybeNotFound(err error) (error, bool) {
	if err != nil && errors.IsNotFound(err) {
		return &environs.NotFoundError{err}, true
	}
	return err, false
}
