package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	DefaultInstancesRoot = ".odoonoir"
	InstancesJSON        = "instances.json"
	ConfigFile           = "config.json"
)

// Config holds global odoonoir settings persisted in ~/.odoonoir/config.json.
type Config struct {
	InstancesRoot string `mapstructure:"instances_root"`
	PostgresUser  string `mapstructure:"postgres_user"`
	PostgresHost  string `mapstructure:"postgres_host"`
	PostgresPort  int    `mapstructure:"postgres_port"`
	OdooUser      string `mapstructure:"odoo_user"`
	OdooPassword  string `mapstructure:"odoo_password"`
	WebhookURL    string `mapstructure:"webhook_url"`
}

// AppDir returns the odoonoir config directory (~/.odoonoir or $ODOONOIR_HOME).
func AppDir() (string, error) {
	if env := os.Getenv("ODOONOIR_HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home dir: %w", err)
	}
	return filepath.Join(home, DefaultInstancesRoot), nil
}

func configFilePath() (string, error) {
	dir, err := AppDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFile), nil
}

func viperFor() (*viper.Viper, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, err
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("json")
	v.SetDefault("instances_root", filepath.Dir(path))
	v.SetDefault("postgres_host", "localhost")
	v.SetDefault("postgres_port", 5432)
	if err := v.ReadInConfig(); err != nil {
		var nf viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
	}
	return v, nil
}

// Load reads the persisted config, filling defaults for missing fields.
func Load() (*Config, error) {
	v, err := viperFor()
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	dir, err := AppDir()
	if err != nil {
		return nil, err
	}
	if cfg.InstancesRoot == "" {
		cfg.InstancesRoot = dir
	}
	if cfg.PostgresHost == "" {
		cfg.PostgresHost = "localhost"
	}
	if cfg.PostgresPort == 0 {
		cfg.PostgresPort = 5432
	}
	if cfg.OdooUser == "" {
		cfg.OdooUser = "odoo"
	}
	return cfg, nil
}

// Save persists the config file, creating the app dir if needed.
func Save(cfg *Config) error {
	dir, err := AppDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create app dir: %w", err)
	}
	path, err := configFilePath()
	if err != nil {
		return err
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("json")
	v.Set("instances_root", cfg.InstancesRoot)
	v.Set("postgres_user", cfg.PostgresUser)
	v.Set("postgres_host", cfg.PostgresHost)
	v.Set("postgres_port", cfg.PostgresPort)
	v.Set("odoo_user", cfg.OdooUser)
	v.Set("odoo_password", cfg.OdooPassword)
	v.Set("webhook_url", cfg.WebhookURL)
	all := v.AllSettings()
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// InstancesDir returns the directory holding all instances.
func (c *Config) InstancesDir() string { return c.InstancesRoot }

// Path returns the full path for a named instance.
func (c *Config) InstancePath(name string) string {
	return filepath.Join(c.InstancesDir(), name)
}

// RegistryPath is the path to instances.json.
func (c *Config) RegistryPath() string {
	return filepath.Join(c.InstancesDir(), InstancesJSON)
}
