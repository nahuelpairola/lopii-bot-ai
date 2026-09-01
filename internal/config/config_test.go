package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestEveryConfigFile_HasNoSameTurnModelCollision(t *testing.T) {
	archivos, err := filepath.Glob(filepath.Join("..", "..", "config", "*.toml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(archivos) == 0 {
		t.Fatal("no se encontró ningún config/*.toml: el test no está mirando nada")
	}
	for _, ruta := range archivos {
		t.Run(filepath.Base(ruta), func(t *testing.T) {
			v := viper.New()
			v.SetConfigFile(ruta)
			v.SetConfigType("toml")
			applyDefaults(v)
			if err := v.ReadInConfig(); err != nil {
				t.Fatalf("leer %s: %v", ruta, err)
			}
			var cfg Config
			if err := v.Unmarshal(&cfg); err != nil {
				t.Fatalf("unmarshal %s: %v", ruta, err)
			}
			if conflictos := ModelBucketConflicts(cfg.Groq); len(conflictos) > 0 {
				t.Errorf("%s tiene llamadas del mismo turno compartiendo modelo:\n  %s",
					ruta, strings.Join(conflictos, "\n  "))
			}
		})
	}
}

func TestModelBucketConflicts_DetectsACollision(t *testing.T) {
	g := groq{
		AgentModel:      "modelo-x",
		CreateModel:     "modelo-x",
		QueryModel:      "modelo-y",
		ClassifierModel: "modelo-z",
	}
	conflictos := ModelBucketConflicts(g)
	if len(conflictos) != 1 {
		t.Fatalf("conflictos = %v, want exactamente 1 (agent y create)", conflictos)
	}
	if !strings.Contains(conflictos[0], "agent") || !strings.Contains(conflictos[0], "create") {
		t.Errorf("el conflicto tiene que nombrar los dos lados: %q", conflictos[0])
	}
}

func TestModelBucketConflicts_IgnoresCallsThatNeverShareATurn(t *testing.T) {
	g := groq{
		AgentModel:      "a",
		CreateModel:     "compartido",
		UpdateModel:     "compartido",
		QueryModel:      "q",
		ClassifierModel: "c",
	}
	if conflictos := ModelBucketConflicts(g); len(conflictos) != 0 {
		t.Errorf("update y create no ocurren en el mismo turno: %v", conflictos)
	}
}
