package viper

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/aws/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogWriter is a silent LogWriter for loader tests.
type testLogWriter struct{}

func (testLogWriter) Debug(...any)          {}
func (testLogWriter) Debugf(string, ...any) {}
func (testLogWriter) Info(...any)           {}
func (testLogWriter) Infof(string, ...any)  {}
func (testLogWriter) Warn(...any)           {}
func (testLogWriter) Warnf(string, ...any)  {}
func (testLogWriter) Error(...any)          {}
func (testLogWriter) Errorf(string, ...any) {}
func (testLogWriter) Fatal(...any)          {}
func (testLogWriter) Fatalf(string, ...any) {}
func (l testLogWriter) WithField(string, any) logger.LogWriter {
	return l
}
func (l testLogWriter) WithFields(logrus.Fields) logger.LogWriter {
	return l
}

// confDir writes application.yaml into a temp dir and points CONF_DIR at it.
func confDir(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "application.yaml"), []byte(body), 0o600))
	t.Setenv("CONF_DIR", dir)
	t.Setenv("SCOPE", "local")

	return dir
}

func TestApply_LoadsAndValidates(t *testing.T) {
	confDir(t, `
router:
  port: "9090"
log:
  level: "warn"
  format: "json"
`)

	cfg, err := NewService(testLogWriter{}).Apply()
	require.NoError(t, err)

	assert.Equal(t, "9090", cfg.Router.Port)
	assert.Equal(t, "warn", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
}

func TestApply_MissingFileIsAnError(t *testing.T) {
	t.Setenv("CONF_DIR", t.TempDir())
	t.Setenv("SCOPE", "local")

	_, err := NewService(testLogWriter{}).Apply()
	assert.Error(t, err, "a missing application.yaml must not load silently")
}

func TestApply_InvalidConfigFailsValidation(t *testing.T) {
	confDir(t, `
aws:
  region: "not a region!!"
sqs_clients:
  - orders:
      endpoint: "http://localhost:4566"
`)

	_, err := NewService(testLogWriter{}).Apply()
	require.Error(t, err)
	assert.ErrorContains(t, err, "validation failed")
}

func TestApply_AWSRegionOnlyRequiredWhenUsed(t *testing.T) {
	confDir(t, `
router:
  port: "8080"
`)

	cfg, err := NewService(testLogWriter{}).Apply()
	require.NoError(t, err, "a service with no AWS adapter must not need a region")
	assert.False(t, UsesAWS(cfg))
}

func TestApplyDynamic_BuildsAndValidates(t *testing.T) {
	confDir(t, `
enable_config_watch: true
router:
  port: "8080"
feature_flags:
  beta: true
`)

	dc, err := NewService(testLogWriter{}).ApplyDynamic(nil)
	require.NoError(t, err)
	require.NotNil(t, dc)

	cfg, ok := dc.Get().(*Config)
	require.True(t, ok)
	assert.Equal(t, true, cfg.FeatureFlags["beta"])
}

func TestResolveEnvValue(t *testing.T) {
	t.Setenv("GO_ENGINE_TEST_VALUE", "from-env")

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain value passes through", "literal", "literal"},
		{"env var is resolved", "${GO_ENGINE_TEST_VALUE}", "from-env"},
		{"missing env falls back to default", "${GO_ENGINE_MISSING:-fallback}", "fallback"},
		{"missing env with no default is left as is", "${GO_ENGINE_MISSING}", "${GO_ENGINE_MISSING}"},
		{"set env wins over default", "${GO_ENGINE_TEST_VALUE:-fallback}", "from-env"},
		{"unclosed placeholder is literal", "${GO_ENGINE_TEST_VALUE", "${GO_ENGINE_TEST_VALUE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveEnvValue(tc.input))
		})
	}
}

func TestApply_ResolvesEnvPlaceholders(t *testing.T) {
	t.Setenv("GO_ENGINE_TEST_PORT", "7777")
	confDir(t, `
router:
  port: "${GO_ENGINE_TEST_PORT}"
`)

	cfg, err := NewService(testLogWriter{}).Apply()
	require.NoError(t, err)
	assert.Equal(t, "7777", cfg.Router.Port)
}

func TestGetMissingFiles(t *testing.T) {
	assert.Empty(t, getMissingFiles([]string{"a.yaml"}, []string{"a.yaml", "b.yaml"}))
	assert.Equal(t, []string{"c.yaml"}, getMissingFiles([]string{"a.yaml", "c.yaml"}, []string{"a.yaml"}))
	assert.Empty(t, getMissingFiles(nil, []string{"a.yaml"}))
}

func TestContains(t *testing.T) {
	assert.True(t, contains([]string{"a", "b"}, "b"))
	assert.False(t, contains([]string{"a", "b"}, "c"))
	assert.False(t, contains(nil, "a"))
}

func TestGetConfigPath(t *testing.T) {
	t.Run("CONF_DIR wins", func(t *testing.T) {
		t.Setenv("CONF_DIR", "/custom/path")
		assert.Equal(t, "/custom/path", getConfigPath(testLogWriter{}))
	})

	t.Run("local profile uses config/", func(t *testing.T) {
		t.Setenv("CONF_DIR", "")
		t.Setenv("SCOPE", "local")
		assert.Equal(t, "config", getConfigPath(testLogWriter{}))
	})
}

func TestValidateConfig_ReportsEveryProblem(t *testing.T) {
	errs := ValidateConfig(Config{
		Aws:        AwsConfig{Region: "bad region"},
		SQSClients: []map[string]sqs.Config{{"q": {}}},
		Router:     router.Config{},
	})
	assert.NotEmpty(t, errs)
}

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{Field: "aws.region", Message: "is required"}
	assert.Contains(t, e.Error(), "aws.region")
	assert.Contains(t, e.Error(), "is required")
}
