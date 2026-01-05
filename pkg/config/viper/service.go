package viper

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/config/dynamic"
	"github.com/skolldire/go-engine/pkg/utilities/app_profile"
	"github.com/skolldire/go-engine/pkg/utilities/file_utils"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/spf13/viper"
)

func NewService(logger *logrus.Logger) Service {
	once.Do(func() {
		instance = &service{
			propertyFiles: getPropertyFiles(logger),
			path:          getConfigPath(logger),
			log:           logger,
		}
	})
	return instance
}

func (s *service) Apply() (Config, error) {
	if err := s.validateRequiredFiles(); err != nil {
		s.log.Error("error validating configuration files: ", err)
		return Config{}, err
	}

	mergedConfig, err := s.loadAndMergeConfigs()
	if err != nil {
		s.log.Error("error loading configuration: ", err)
		return Config{}, fmt.Errorf("error loading configuration - %w", err)
	}

	config, err := s.mapConfigToStruct(mergedConfig)
	if err != nil {
		s.log.Error("error mapping configuration: ", err)
		return Config{}, fmt.Errorf("error mapping configuration - %w", err)
	}

	// Validate configuration structure
	if validationErrors := ValidateConfig(config); len(validationErrors) > 0 {
		var errorMessages []string
		for _, err := range validationErrors {
			errorMessages = append(errorMessages, err.Error())
			s.log.Error("configuration validation error: ", err)
		}
		return Config{}, fmt.Errorf("configuration validation failed: %s", strings.Join(errorMessages, "; "))
	}

	s.log.Info("configuration loaded and validated successfully")
	return config, nil
}

func (s *service) validateRequiredFiles() error {
	files, err := file_utils.ListFiles(s.path)
	if err != nil {
		s.log.Errorf("Error listando archivos en %s: %v", s.path, err)
		return err
	}

	missingFiles := getMissingFiles(s.propertyFiles, files)
	if len(missingFiles) > 0 {
		s.log.Errorf("missing configuration files: %v", missingFiles)
		return fmt.Errorf("missing configuration files: %v", missingFiles)
	}

	s.log.Debug("all required configuration files are present")
	return nil
}

func (s *service) loadAndMergeConfigs() (*viper.Viper, error) {
	v := viper.New()
	v.AddConfigPath(s.path)
	v.SetConfigType("yaml")

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	baseFileName := "application"
	if err := loadConfigFile(v, s.path, baseFileName, s.log); err != nil {
		return nil, fmt.Errorf("error loading base configuration: %w", err)
	}

	envFileName := s.getPropertyFileName()
	if envFileName != baseFileName {
		envV := viper.New()
		envV.SetConfigType("yaml")

		if err := loadConfigFile(envV, s.path, envFileName, s.log); err != nil {
			s.log.Warnf("failed to load environment-specific configuration %s: %v", envFileName, err)
		} else {
			if err := v.MergeConfigMap(envV.AllSettings()); err != nil {
				return nil, fmt.Errorf("failed to merge configurations: %w", err)
			}
		}
	}

	if v.GetBool("enable_config_watch") {
		watchConfig(v, s.log)
	}

	s.log.Debug("configuration files merged successfully")
	return v, nil
}

func (s *service) mapConfigToStruct(v *viper.Viper) (Config, error) {
	var config Config

	decoderConfig := &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           &config,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			envVarDecodeHook(),
		),
		MatchName: func(mapKey, fieldName string) bool {
			snakeToField := strings.Replace(mapKey, "_", "", -1)
			fieldToSnake := strings.ToLower(fieldName)
			return strings.EqualFold(snakeToField, fieldToSnake) ||
				strings.EqualFold(mapKey, fieldName) ||
				strings.EqualFold(snakeToField, fieldToSnake)
		},
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return Config{}, fmt.Errorf("error creating decoder: %w", err)
	}

	if err := decoder.Decode(v.AllSettings()); err != nil {
		s.log.Error("error decoding configuration: ", err)
		return Config{}, err
	}

	s.log.Debug("configuration mapped successfully to structure")
	return config, nil
}

func (s *service) ApplyDynamic(log logger.Service) (*dynamic.DynamicConfig, error) {
	mergedConfig, err := s.loadAndMergeConfigs()
	if err != nil {
		return nil, fmt.Errorf("error loading initial configuration: %w", err)
	}

	config, err := s.mapConfigToStruct(mergedConfig)
	if err != nil {
		return nil, fmt.Errorf("error mapping configuration: %w", err)
	}

	dynamicConfig := dynamic.NewDynamicConfig(&config, log)

	dynamicConfig.SetReloadFunc(func() (interface{}, error) {
		mergedConfig, err := s.loadAndMergeConfigs()
		if err != nil {
			return nil, err
		}
		config, err := s.mapConfigToStruct(mergedConfig)
		if err != nil {
			return nil, err
		}
		return &config, nil
	})

	if mergedConfig.GetBool("enable_config_watch") {
		configPath := s.path
		configFiles := []string{
			filepath.Join(configPath, "application.yaml"),
		}

		envFileName := s.getPropertyFileName()
		if envFileName != "application" {
			configFiles = append(configFiles, filepath.Join(configPath, envFileName+".yaml"))
		}

		fileWatcher, err := dynamic.NewFileWatcher(configFiles, log)
		if err != nil {
			s.log.Warnf("failed to create file watcher: %v", err)
		} else {
			dynamicConfig.AddWatcher(fileWatcher)
		}
	}

	s.log.Info("dynamic configuration created successfully")
	return dynamicConfig, nil
}

func (s *service) getPropertyFileName() string {
	scopeFile := fmt.Sprintf("application-%s", app_profile.GetScopeValue())
	profileFile := fmt.Sprintf("application-%s", app_profile.GetProfileByScope())

	files, err := file_utils.ListFiles(s.path)
	if err != nil {
		s.log.Warnf("error listing files to search for specific configurations: %v", err)
		return "application"
	}

	for _, file := range files {
		if strings.HasPrefix(file, scopeFile+".") {
			return scopeFile
		}
	}

	for _, file := range files {
		if strings.HasPrefix(file, profileFile+".") {
			return profileFile
		}
	}

	s.log.Warnf("specific configuration file not found for %s or %s", scopeFile, profileFile)
	return "application"
}

// loadConfigFile loads the named configuration file from the given path into v.
// It sets the Viper config path and name, attempts to read the configuration, and
// returns any error returned by v.ReadInConfig. If the file is not found a warning
// is logged; other read errors are logged as errors.
func loadConfigFile(v *viper.Viper, path, name string, logger *logrus.Logger) error {
	v.AddConfigPath(path)
	v.SetConfigName(name)

	logger.Debugf("attempting to load configuration file: %s in %s", name, path)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			logger.Warnf("configuration file not found: %s", name)
		} else {
			logger.Errorf("error reading configuration file %s: %v", name, err)
		}
		return err
	}

	logger.Infof("configuration file %s loaded successfully", name)
	return nil
}

func envVarDecodeHook() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}

		str, ok := data.(string)
		if !ok {
			return data, nil
		}

		return resolveEnvValue(str), nil
	}
}

// watchConfig enables watching v's configuration file and logs a warning with the changed file name when a change is detected.
func watchConfig(v *viper.Viper, logger *logrus.Logger) {
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		logger.Warnf("configuration file changed: %s", e.Name)
	})
}

// getPropertyFiles determines the list of required configuration filenames based on
// the application's profile and which files exist in the configuration directory.
// It always includes "application.yaml" and, if present in the config path, appends
// the scope-specific file (application-<scope>.yaml) or, if that is absent, the
// profile-specific file (application-<profile>.yaml).
// If the configuration directory cannot be listed, it returns only "application.yaml".
func getPropertyFiles(logger *logrus.Logger) []string {
	requiredFiles := []string{"application.yaml"}
	scopeFile := fmt.Sprintf("application-%s.yaml", app_profile.GetScopeValue())
	profileFile := fmt.Sprintf("application-%s.yaml", app_profile.GetProfileByScope())

	path := getConfigPath(logger)
	availableFiles, err := file_utils.ListFiles(path)
	if err != nil {
		logger.Warnf("failed to list configuration files: %v", err)
		return requiredFiles
	}

	if contains(availableFiles, scopeFile) {
		requiredFiles = append(requiredFiles, scopeFile)
	} else if contains(availableFiles, profileFile) {
		requiredFiles = append(requiredFiles, profileFile)
	}

	logger.Debugf("Archivos requeridos: %v", requiredFiles)
	return requiredFiles
}

// getConfigPath determines the directory path used for configuration files.
// It uses the CONF_DIR environment variable if set; otherwise it returns "config"
// when the application is running with a local profile, and "/app/config" in all other cases.
func getConfigPath(logger *logrus.Logger) string {
	if path := os.Getenv("CONF_DIR"); path != "" {
		logger.Debugf("using CONF_DIR: %s", path)
		return path
	}

	if app_profile.IsLocalProfile() {
		logger.Debug("using local profile for configuration")
		return "config"
	}

	logger.Debug("using default configuration in /app/config")
	return "/app/config"
}

func getMissingFiles(required, available []string) []string {
	var missing []string
	for _, file := range required {
		if !contains(available, file) {
			missing = append(missing, file)
		}
	}
	return missing
}

func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func resolveEnvValue(value string) string {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
		parts := strings.SplitN(trimmed, ":-", 2)
		envValue := os.Getenv(parts[0])

		if envValue != "" {
			return envValue
		}
		if len(parts) > 1 {
			return parts[1]
		}
	}
	return value
}