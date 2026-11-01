package pgtool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Server is a throwaway PostgreSQL cluster on a unix socket.
type Server struct {
	bin     string
	dir     string
	sock    string
	Major   int
	asRoot  bool
	DBName  string
	stopped bool
}

// FindBin locates PostgreSQL server binaries: $PGBIN, the newest
// /usr/lib/postgresql/*/bin, or the PATH.
func FindBin() (string, error) {
	if p := os.Getenv("PGBIN"); p != "" {
		return p, nil
	}
	matches, _ := filepath.Glob("/usr/lib/postgresql/*/bin")
	sort.Slice(matches, func(i, j int) bool { return versionOf(matches[i]) < versionOf(matches[j]) })
	if len(matches) > 0 {
		return matches[len(matches)-1], nil
	}
	if p, err := exec.LookPath("initdb"); err == nil {
		return filepath.Dir(p), nil
	}
	return "", fmt.Errorf("no PostgreSQL server binaries found (set PGBIN)")
}

func versionOf(p string) int {
	m := regexp.MustCompile(`/(\d+)/bin$`).FindStringSubmatch(p)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func (s *Server) run(name string, args ...string) ([]byte, error) {
	full := filepath.Join(s.bin, name)
	var cmd *exec.Cmd
	if s.asRoot {
		cmd = exec.Command("runuser", append([]string{"-u", "postgres", "--", full}, args...)...)
	} else {
		cmd = exec.Command(full, args...)
	}
	return cmd.CombinedOutput()
}

// StartEphemeral initialises and starts a cluster listening only on a unix
// socket, with fsync off. Call Close when done.
func StartEphemeral(bin string) (*Server, error) {
	if bin == "" {
		var err error
		if bin, err = FindBin(); err != nil {
			return nil, err
		}
	}
	dir, err := os.MkdirTemp("", "qlx-pg-")
	if err != nil {
		return nil, err
	}
	s := &Server{bin: bin, dir: dir, sock: filepath.Join(dir, "sock"), asRoot: os.Geteuid() == 0, DBName: "qlx"}
	if err := os.MkdirAll(s.sock, 0o755); err != nil {
		return nil, err
	}
	if s.asRoot {
		_ = exec.Command("chown", "-R", "postgres", dir).Run()
		_ = os.Chmod(dir, 0o755)
	}
	if out, err := s.run("postgres", "--version"); err == nil {
		if m := regexp.MustCompile(`(\d+)`).FindString(string(out)); m != "" {
			s.Major, _ = strconv.Atoi(m)
		}
	}
	data := filepath.Join(dir, "data")
	if out, err := s.run("initdb", "-D", data, "-A", "trust", "-E", "UTF8"); err != nil {
		s.Close()
		return nil, fmt.Errorf("initdb: %v: %s", err, out)
	}
	opts := fmt.Sprintf("-c listen_addresses='' -c unix_socket_directories='%s' -c fsync=off", s.sock)
	if out, err := s.run("pg_ctl", "-D", data, "-w", "-l", filepath.Join(dir, "log"), "-o", opts, "start"); err != nil {
		s.Close()
		return nil, fmt.Errorf("pg_ctl start: %v: %s", err, out)
	}
	if out, err := s.run("createdb", "-h", s.sock, s.DBName); err != nil {
		s.Close()
		return nil, fmt.Errorf("createdb: %v: %s", err, out)
	}
	return s, nil
}

// Conn returns a psql connection to the server's database.
func (s *Server) Conn() Conn {
	env := []string{"PGHOST=" + s.sock, "PGDATABASE=" + s.DBName}
	if s.asRoot {
		env = append(env, "PGUSER=postgres")
	}
	return Conn{Env: env}
}

// Close stops the server and removes its files.
func (s *Server) Close() {
	if s.stopped {
		return
	}
	s.stopped = true
	_, _ = s.run("pg_ctl", "-D", filepath.Join(s.dir, "data"), "-m", "immediate", "stop")
	_ = os.RemoveAll(s.dir)
}

// ApplySQLDir applies every *.sql file in dir, in name order.
func (c Conn) ApplySQLDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no .sql files in %s", dir)
	}
	for _, f := range files {
		if err := c.ExecFile(f); err != nil {
			return err
		}
	}
	return nil
}

var _ = strings.TrimSpace
