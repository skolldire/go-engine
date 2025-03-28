package viper

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/utilities/app_profile"
	"github.com/skolldire/go-engine/pkg/utilities/file_utils"
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
		s.log.Error("Error validando archivos de configuración: ", err)
		return Config{}, err
	}

	mergedConfig, err := s.loadAndMergeConfigs()
	if err != nil {
		s.log.Error("Error cargando configuración: ", err)
		return Config{}, fmt.Errorf("error loading configuration - %w", err)
	}

	s.log.Info("Configuración cargada correctamente")
	return s.mapConfigToStruct(mergedConfig)
}

func (s *service) validateRequiredFiles() error {
	files, err := file_utils.ListFiles(s.path)
	if err != nil {
		s.log.Errorf("Error listando archivos en %s: %v", s.path, err)
		return err
	}

	missingFiles := getMissingFiles(s.propertyFiles, files)
	if len(missingFiles) > 0 {
		s.log.Errorf("Archivos de configuración faltantes: %v", missingFiles)
		return fmt.Errorf("faltan archivos de configuración: %v", missingFiles)
	}

	s.log.Debug("Todos los archivos de configuración requeridos están presentes")
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
			s.log.Warnf("No se pudo cargar la configuración específica del entorno %s: %v", envFileName, err)
		} else {
			if err := v.MergeConfigMap(envV.AllSettings()); err != nil {
				return nil, fmt.Errorf("failed to merge configurations: %w", err)
			}
		}
	}

	if v.GetBool("enable_config_watch") {
		watchConfig(v, s.log)
	}

	s.log.Debug("Archivos de configuración combinados correctamente")
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
		return Config{}, fmt.Errorf("error creando decodificador: %w", err)
	}

	if err := decoder.Decode(v.AllSettings()); err != nil {
		s.log.Error("Error decodificando configuración: ", err)
		return Config{}, err
	}

	s.log.Debug("Configuración mapeada correctamente a la estructura")
	return config, nil
}

func (s *service) getPropertyFileName() string {
	scopeFile := fmt.Sprintf("application-%s", app_profile.GetScopeValue())
	profileFile := fmt.Sprintf("application-%s", app_profile.GetProfileByScope())

	files, err := file_utils.ListFiles(s.path)
	if err != nil {
		s.log.Warnf("Error listando archivos para buscar configuraciones específicas: %v", err)
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

	s.log.Warnf("No se encontró archivo de configuración específico para %s o %s", scopeFile, profileFile)
	return "application"
}

func loadConfigFile(v *viper.Viper, path, name string, logger *logrus.Logger) error {
	v.AddConfigPath(path)
	v.SetConfigName(name)

	logger.Debugf("Intentando cargar archivo de configuración: %s en %s", name, path)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			logger.Warnf("Archivo de configuración no encontrado: %s", name)
		} else {
			logger.Errorf("Error leyendo archivo de configuración %s: %v", name, err)
		}
		return err
	}

	logger.Infof("Archivo de configuración %s cargado correctamente", name)
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

func watchConfig(v *viper.Viper, logger *logrus.Logger) {
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		logger.Warnf("Archivo de configuración cambiado: %s", e.Name)
	})
}

func getPropertyFiles(logger *logrus.Logger) []string {
	requiredFiles := []string{"application.yaml"}
	scopeFile := fmt.Sprintf("application-%s.yaml", app_profile.GetScopeValue())
	profileFile := fmt.Sprintf("application-%s.yaml", app_profile.GetProfileByScope())

	path := getConfigPath(logger)
	availableFiles, err := file_utils.ListFiles(path)
	if err != nil {
		logger.Warnf("No se pudieron listar archivos de configuración: %v", err)
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

func getConfigPath(logger *logrus.Logger) string {
	if path := os.Getenv("CONF_DIR"); path != "" {
		logger.Debugf("Usando CONF_DIR: %s", path)
		return path
	}

	if app_profile.IsLocalProfile() {
		logger.Debug("Usando perfil local para configuración")
		return "config"
	}

	logger.Debug("Usando configuración por defecto en /app/config")
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
