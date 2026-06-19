// Package config загружает конфигурацию из YAML-файла с переопределением
// через переменные окружения (префикс APP_, разделитель уровней — "__").
package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTP      HTTPConfig      `yaml:"http"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Redis     RedisConfig     `yaml:"redis"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"ratelimit"`
	Email     EmailConfig     `yaml:"email"`
}

type HTTPConfig struct {
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type MySQLConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time"`
}

type RedisConfig struct {
	Addr        string        `yaml:"addr"`
	Password    string        `yaml:"password"`
	DB          int           `yaml:"db"`
	TaskListTTL time.Duration `yaml:"task_list_ttl"`
}

type AuthConfig struct {
	JWTSecret string        `yaml:"jwt_secret"`
	TokenTTL  time.Duration `yaml:"token_ttl"`
}

type RateLimitConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
}

type EmailConfig struct {
	FailureThreshold int           `yaml:"failure_threshold"`
	OpenTimeout      time.Duration `yaml:"open_timeout"`
	FailRate         float64       `yaml:"fail_rate"`
}

// Load читает YAML по пути path и накладывает переопределения из ENV.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}
	if err := applyEnvOverrides(reflect.ValueOf(cfg).Elem(), "APP"); err != nil {
		return nil, fmt.Errorf("apply env overrides: %w", err)
	}
	return cfg, nil
}

// applyEnvOverrides рекурсивно проходит структуру; для каждого поля строит
// имя переменной окружения по yaml-тегам (APP_HTTP__PORT и т.п.).
func applyEnvOverrides(v reflect.Value, prefix string) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("yaml")
		tag = strings.Split(tag, ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		envName := prefix + "_" + strings.ToUpper(tag)
		fv := v.Field(i)

		if fv.Kind() == reflect.Struct && fv.Type() != reflect.TypeOf(time.Duration(0)) {
			if err := applyEnvOverrides(fv, envName); err != nil {
				return err
			}
			continue
		}

		raw, ok := os.LookupEnv(envName)
		if !ok {
			continue
		}
		if err := setValue(fv, raw); err != nil {
			return fmt.Errorf("set %s: %w", envName, err)
		}
	}
	return nil
}

func setValue(fv reflect.Value, raw string) error {
	switch fv.Interface().(type) {
	case time.Duration:
		d, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		fv.Set(reflect.ValueOf(d))
		return nil
	}

	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	default:
		return fmt.Errorf("unsupported kind %s", fv.Kind())
	}
	return nil
}
