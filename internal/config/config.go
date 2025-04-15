package config

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/pkg/errors"
)

const (
	inputDirName  = "input"
	jobsDirName   = "jobs"
	resultDirName = "output"
)

type (
	Config struct {
		BotApiKey string `yaml:"bot_api_key"`
		Debug     bool   `yaml:"debug"`
		Paths     Paths
	}
	Paths struct {
		Input  string
		Jobs   string
		Result string
	}
)

func NewConfig(cfgFolderPath string) (*Config, error) {
	const errMsg = "Config.NewConfig"

	c := &Config{
		Paths: Paths{
			Input:  inputDirName,
			Jobs:   jobsDirName,
			Result: resultDirName,
		},
	}

	envPath := filepath.Join(cfgFolderPath, "app.env")

	err := c.loadEnv(envPath)
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}

	err = c.validate()
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}

	return c, nil
}

func (c *Config) loadEnv(filePath string) error {
	err := godotenv.Load(filePath)
	if err != nil {
		return errors.Wrap(err, "loadEnv")
	}

	c.BotApiKey = os.Getenv("bot_api_key")
	c.Debug, _ = strconv.ParseBool(os.Getenv("debug"))

	return nil
}

func (c *Config) validate() error {
	if c.BotApiKey == "" {
		err := errors.New("bot_api_key is required")

		return errors.Wrap(err, "validate")
	}

	return nil
}
