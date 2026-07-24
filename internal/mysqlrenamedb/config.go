package mysqlrenamedb

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func loadFileConfig(path string) (FileConfig, error) {
	if strings.TrimSpace(path) == "" {
		return FileConfig{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, err
	}
	var config FileConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return FileConfig{}, err
	}
	return config, nil
}

func loadMyCnf(path string) (DefaultsFileConfig, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultsFileConfig{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return DefaultsFileConfig{}, err
	}
	return parseMyCnf(string(content)), nil
}

func loadAutoDefaultsFile() (DefaultsFileConfig, string, error) {
	candidates := []string{"/etc/my.cnf", "/etc/mysql/my.cnf"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".my.cnf"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		config, err := loadMyCnf(candidate)
		return config, candidate, err
	}
	return DefaultsFileConfig{}, "", nil
}

func parseMyCnf(content string) DefaultsFileConfig {
	config := DefaultsFileConfig{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	inClient := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			inClient = section == "client"
			continue
		}
		if !inClient {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user":
			config.User = value
		case "password":
			config.Password = value
		case "host":
			config.Host = value
		case "socket":
			config.Socket = value
		case "port":
			if port, err := strconv.Atoi(value); err == nil {
				config.Port = port
			}
		}
	}
	return config
}

func mergeCredentials(cli DefaultsFileConfig, defaultsFile DefaultsFileConfig, fileConfig FileConfig) DefaultsFileConfig {
	merged := DefaultsFileConfig{
		User:     fileConfig.DefaultUser,
		Password: fileConfig.DefaultPassword,
	}
	if defaultsFile.User != "" {
		merged.User = defaultsFile.User
	}
	if defaultsFile.Password != "" {
		merged.Password = defaultsFile.Password
	}
	if defaultsFile.Host != "" {
		merged.Host = defaultsFile.Host
	}
	if defaultsFile.Port != 0 {
		merged.Port = defaultsFile.Port
	}
	if defaultsFile.Socket != "" {
		merged.Socket = defaultsFile.Socket
	}
	if cli.User != "" {
		merged.User = cli.User
	}
	if cli.Password != "" {
		merged.Password = cli.Password
	}
	if cli.Host != "" {
		merged.Host = cli.Host
	}
	if cli.Port != 0 {
		merged.Port = cli.Port
	}
	if cli.Socket != "" {
		merged.Socket = cli.Socket
	}
	return merged
}
