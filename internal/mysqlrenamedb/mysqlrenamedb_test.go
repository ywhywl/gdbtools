package mysqlrenamedb

import (
	"testing"
)

func TestConnectionConfigDisplayName(t *testing.T) {
	tests := []struct {
		name   string
		config ConnectionConfig
		want   string
	}{
		{
			name: "with label",
			config: ConnectionConfig{
				Host:     "192.168.1.100",
				Port:     3306,
				User:     "root",
				Password: "secret",
				Label:    "production",
			},
			want: "production",
		},
		{
			name: "with socket",
			config: ConnectionConfig{
				User:     "root",
				Password: "secret",
				Socket:   "/var/run/mysqld/mysqld.sock",
			},
			want: "root@/var/run/mysqld/mysqld.sock",
		},
		{
			name: "with host and port",
			config: ConnectionConfig{
				Host:     "192.168.1.100",
				Port:     3306,
				User:     "admin",
				Password: "secret",
			},
			want: "admin@192.168.1.100:3306",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.DisplayName()
			if got != tt.want {
				t.Errorf("DisplayName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseMyCnf(t *testing.T) {
	content := `
# This is a comment
[client]
user = testuser
password = testpass
host = localhost
port = 3307
socket = /tmp/mysql.sock

[mysqld]
bind-address = 0.0.0.0
`
	config := parseMyCnf(content)

	if config.User != "testuser" {
		t.Errorf("User = %v, want testuser", config.User)
	}
	if config.Password != "testpass" {
		t.Errorf("Password = %v, want testpass", config.Password)
	}
	if config.Host != "localhost" {
		t.Errorf("Host = %v, want localhost", config.Host)
	}
	if config.Port != 3307 {
		t.Errorf("Port = %v, want 3307", config.Port)
	}
	if config.Socket != "/tmp/mysql.sock" {
		t.Errorf("Socket = %v, want /tmp/mysql.sock", config.Socket)
	}
}

func TestMergeCredentials(t *testing.T) {
	cli := DefaultsFileConfig{
		User: "cli_user",
		Port: 3308,
	}
	defaultsFile := DefaultsFileConfig{
		User:     "defaults_user",
		Password: "defaults_pass",
		Host:     "defaults_host",
		Port:     3307,
	}
	fileConfig := FileConfig{
		DefaultUser:     "file_user",
		DefaultPassword: "file_pass",
	}

	merged := mergeCredentials(cli, defaultsFile, fileConfig)

	// CLI should override everything
	if merged.User != "cli_user" {
		t.Errorf("User = %v, want cli_user (CLI should override)", merged.User)
	}
	if merged.Port != 3308 {
		t.Errorf("Port = %v, want 3308 (CLI should override)", merged.Port)
	}
	// Defaults file should override file config
	if merged.Password != "defaults_pass" {
		t.Errorf("Password = %v, want defaults_pass", merged.Password)
	}
	if merged.Host != "defaults_host" {
		t.Errorf("Host = %v, want defaults_host", merged.Host)
	}
}

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name    string
		config  ConnectionConfig
		timeout int
		want    string
	}{
		{
			name: "tcp connection",
			config: ConnectionConfig{
				Host:     "192.168.1.100",
				Port:     3306,
				User:     "root",
				Password: "secret",
			},
			timeout: 5,
			want:    "root:secret@tcp(192.168.1.100:3306)/?timeout=5s&parseTime=true",
		},
		{
			name: "unix socket",
			config: ConnectionConfig{
				User:     "root",
				Password: "secret",
				Socket:   "/var/run/mysqld/mysqld.sock",
			},
			timeout: 10,
			want:    "root:secret@unix(/var/run/mysqld/mysqld.sock)/?timeout=10s&parseTime=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDSN(tt.config, tt.timeout)
			if got != tt.want {
				t.Errorf("buildDSN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseArgs_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing host and socket",
			args:    []string{"--old-dbname", "test", "--new-dbname", "test2"},
			wantErr: "--host or --socket is required",
		},
		{
			name:    "missing old-dbname",
			args:    []string{"--host", "localhost", "--new-dbname", "test2"},
			wantErr: "--old-dbname is required",
		},
		{
			name:    "missing new-dbname",
			args:    []string{"--host", "localhost", "--old-dbname", "test"},
			wantErr: "--new-dbname is required",
		},
		{
			name:    "same old and new dbname",
			args:    []string{"--host", "localhost", "--old-dbname", "test", "--new-dbname", "test"},
			wantErr: "--old-dbname and --new-dbname cannot be the same",
		},
		{
			name:    "invalid output format",
			args:    []string{"--host", "localhost", "--old-dbname", "test", "--new-dbname", "test2", "--output-format", "xml"},
			wantErr: "invalid --output-format, expected text or json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs() expected error, got nil")
				return
			}
			if err.Error() != tt.wantErr {
				t.Errorf("parseArgs() error = %v, want %v", err.Error(), tt.wantErr)
			}
		})
	}
}
