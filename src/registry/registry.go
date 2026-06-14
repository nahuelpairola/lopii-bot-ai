package registry

import (
	"errors"

	"lopiibot.com/src/config"
)

type AppContainer struct {
}

type registry struct {
	config *config.Config
}

func NewRegistry(cnf *config.Config) (*registry, error) {
	if cnf == nil {
		return nil, errors.New("config must be defined")
	}
	return &registry{
		config: cnf,
	}, nil
}

func (r *registry) InitAppContainer() (*AppContainer, error) {
	_, err := r.initDatabase()
	if err != nil {
		return nil, errors.Join(errors.New("database initialization failed"), err)
	}
	container := AppContainer{}
	return &container, nil
}
