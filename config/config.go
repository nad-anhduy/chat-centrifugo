package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	PostgresDSN    string `mapstructure:"POSTGRES_DSN"`
	ScyllaHosts    string `mapstructure:"SCYLLA_HOSTS"`
	ScyllaKeyspace string `mapstructure:"SCYLLA_KEYSPACE"`
	RedisURL       string `mapstructure:"REDIS_URL"`
	CentrifugoAPI  string `mapstructure:"CENTRIFUGO_API_URL"`
	CentrifugoKey  string `mapstructure:"CENTRIFUGO_API_KEY"`
	JWTSecret      string `mapstructure:"JWT_SECRET"`
	Port           string `mapstructure:"PORT"`
}

func LoadConfig(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	var config Config

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	err := viper.Unmarshal(&config)
	return &config, err
}
