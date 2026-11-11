package runner

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A loopback endpoint keeps poke identical on Unix and Windows. The rendezvous
// file is private, and its lock stays held for the lifetime of the listener.
type endpoint struct {
	Address string `json:"address"`
	Token   string `json:"token"`
}
type control struct {
	listener net.Listener
	lock     *os.File
	path     string
	done     chan struct{}
}

func controlPath(name string) (string, error) {
	dir := os.Getenv("RUNNER_RUNTIME_DIR")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "runner")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, strings.ToLower(name)+".json"), nil
}

func listenControl(name string, triggers chan<- struct{}) (*control, error) {
	file, err := controlPath(name)
	if err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(file+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(lock); err != nil {
		lock.Close()
		return nil, fmt.Errorf("instance %q is already running (or its lock is unavailable): %w", name, err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		lock.Close()
		return nil, err
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		listener.Close()
		lock.Close()
		return nil, err
	}
	e := endpoint{listener.Addr().String(), hex.EncodeToString(secret[:])}
	data, _ := json.Marshal(e)
	// Removing under the exclusive lock also recovers a crashed instance's file.
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		listener.Close()
		lock.Close()
		return nil, err
	}
	if err := os.WriteFile(file, data, 0600); err != nil {
		listener.Close()
		lock.Close()
		return nil, err
	}
	c := &control{listener, lock, file, make(chan struct{})}
	go func() {
		defer close(c.done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(time.Second))
			line, err := bufio.NewReader(io.LimitReader(conn, 256)).ReadString('\n')
			if err == nil && subtle.ConstantTimeCompare([]byte(strings.TrimSuffix(line, "\n")), []byte(e.Token)) == 1 {
				request(triggers)
				fmt.Fprintln(conn, "ok")
			}
			conn.Close()
		}
	}()
	return c, nil
}

func (c *control) Close() {
	c.listener.Close()
	<-c.done
	os.Remove(c.path)
	// Do not unlink the lock: a second process may already have its inode open.
	c.lock.Close()
}

func poke(name string) error {
	file, err := controlPath(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("instance %q is not running: %w", name, err)
	}
	var e endpoint
	if err := json.Unmarshal(data, &e); err != nil {
		return fmt.Errorf("instance %q is not ready; retry the poke", name)
	}
	host, _, err := net.SplitHostPort(e.Address)
	if err != nil || host != "127.0.0.1" || len(e.Token) != 64 {
		return fmt.Errorf("invalid endpoint for %q", name)
	}
	conn, err := net.DialTimeout("tcp4", e.Address, time.Second)
	if err != nil {
		return fmt.Errorf("instance %q is not running; restart it to replace stale state: %w", name, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprintln(conn, e.Token); err != nil {
		return err
	}
	line, err := bufio.NewReader(io.LimitReader(conn, 16)).ReadString('\n')
	if err != nil || line != "ok\n" {
		return fmt.Errorf("instance %q did not acknowledge the poke", name)
	}
	return nil
}
