# Ejemplo de Manejo Seguro de Secrets - Cliente Cognito

Este documento muestra cómo implementar el manejo seguro de `client_secret` usando Viper y encapsulación en el cliente.

## 📋 Principios de Seguridad

1. ✅ **Carga desde Variables de Entorno:** Usar `${VAR_NAME}` en YAML
2. ✅ **Encapsulación:** Secret almacenado en campo privado del cliente
3. ✅ **No Exposición:** No hay getters públicos, no se serializa, no se loggea
4. ✅ **Limpieza:** Limpiar secret de Config después de copiar al cliente

---

## 🔧 Implementación

### 1. Configuración en `entity.go`

```go
package cognito

import (
    "time"
    "github.com/skolldire/go-engine/pkg/utilities/resilience"
)

// Config configuración del cliente Cognito
// Esta struct es pública para que Viper pueda cargarla desde YAML
type Config struct {
    // AWS Configuration
    Region          string `mapstructure:"region" json:"region"`
    UserPoolID      string `mapstructure:"user_pool_id" json:"user_pool_id"`
    ClientID        string `mapstructure:"client_id" json:"client_id"`
    
    // CRÍTICO: client_secret se carga desde variable de entorno usando ${VAR_NAME}
    // json:"-" evita que se serialice en JSON (seguridad)
    // mapstructure permite que Viper lo cargue desde YAML
    ClientSecret    string `mapstructure:"client_secret" json:"-"` // Opcional
    
    // JWT Configuration
    JWKSUrl         string `mapstructure:"jwks_url" json:"jwks_url"`
    TokenExpiration time.Duration `mapstructure:"token_expiration" json:"token_expiration"`
    
    // Resilience
    Resilience      resilience.Config `mapstructure:"resilience" json:"resilience"`
    
    // Timeouts
    Timeout         time.Duration `mapstructure:"timeout" json:"timeout"`
    MaxRetries      int `mapstructure:"max_retries" json:"max_retries"`
    RetryBackoff    time.Duration `mapstructure:"retry_backoff" json:"retry_backoff"`
}
```

---

### 2. Cliente con Secret Privado en `service.go`

```go
package cognito

import (
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
    "github.com/skolldire/go-engine/pkg/utilities/logger"
    "github.com/skolldire/go-engine/pkg/utilities/resilience"
)

// Client implementa Service usando AWS SDK v2
// El secret se almacena en un campo privado (no exportado)
type Client struct {
    // Config completo (pero ClientSecret se limpia después de inicialización)
    config Config
    
    // CRÍTICO: Campo privado (minúscula) - no exportado
    // Este es el único lugar donde se almacena el secret
    clientSecret string
    
    // Cliente AWS SDK
    cognitoClient *cognitoidentityprovider.Client
    
    // Otros campos...
    jwksClient  *jwks.Client
    logger      logger.Service
    resilience  *resilience.Service
}

// NewClient crea una nueva instancia del cliente Cognito
// CRÍTICO: Manejo seguro del secret
func NewClient(cfg Config, log logger.Service) (Service, error) {
    // Validar configuración
    if err := validateConfig(cfg); err != nil {
        return nil, fmt.Errorf("invalid cognito config: %w", err)
    }
    
    // CRÍTICO: Copiar secret a campo privado ANTES de crear el cliente
    clientSecret := cfg.ClientSecret
    
    // CRÍTICO: Limpiar secret de Config para evitar exposición accidental
    // Esto asegura que si alguien accede a cfg.ClientSecret después,
    // no encontrará el valor real
    cfg.ClientSecret = ""
    
    // Crear cliente AWS SDK
    awsCfg, err := config.LoadDefaultConfig(context.Background(),
        config.WithRegion(cfg.Region),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }
    
    cognitoClient := cognitoidentityprovider.NewFromConfig(awsCfg)
    
    // Configurar JWKS URL si no se proporciona
    jwksURL := cfg.JWKSUrl
    if jwksURL == "" {
        jwksURL = fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s/.well-known/jwks.json",
            cfg.Region, cfg.UserPoolID)
    }
    
    // Crear cliente JWKS
    jwksClient := jwks.NewClient(jwksURL)
    
    // Crear servicio de resiliencia
    resilienceSvc := resilience.NewResilienceService(cfg.Resilience, log)
    
    // Crear cliente con secret privado
    client := &Client{
        config:        cfg,           // Config sin secret (ya limpiado)
        clientSecret:  clientSecret, // Secret en campo privado
        cognitoClient: cognitoClient,
        jwksClient:    jwksClient,
        logger:        log,
        resilience:    resilienceSvc,
    }
    
    // Logging seguro: solo indicar si secret está presente, NO el valor
    if clientSecret != "" {
        log.Debug(context.Background(), "Cognito client initialized with client secret",
            map[string]interface{}{
                "user_pool_id": cfg.UserPoolID,
                "client_id":    cfg.ClientID,
                "has_secret":   true, // Solo indicador, no el valor
            })
    } else {
        log.Debug(context.Background(), "Cognito client initialized without client secret",
            map[string]interface{}{
                "user_pool_id": cfg.UserPoolID,
                "client_id":    cfg.ClientID,
                "has_secret":   false,
            })
    }
    
    return client, nil
}

// computeSecretHash calcula el SecretHash necesario para operaciones con ClientSecret
// CRÍTICO: Método privado (minúscula) - solo usado internamente
// No expone el secret ni el hash calculado
func (c *Client) computeSecretHash(username string) string {
    if c.clientSecret == "" {
        return ""
    }
    
    // Fórmula: HMAC_SHA256(clientSecret, username + clientID)
    message := username + c.config.ClientID
    mac := hmac.New(sha256.New, []byte(c.clientSecret))
    mac.Write([]byte(message))
    hash := mac.Sum(nil)
    
    // Base64 encode
    return base64.StdEncoding.EncodeToString(hash)
}

// RegisterUser ejemplo de uso del secret interno
func (c *Client) RegisterUser(ctx context.Context, req RegisterUserRequest) (*User, error) {
    // ... validación ...
    
    input := &cognitoidentityprovider.SignUpInput{
        ClientId:   aws.String(c.config.ClientID),
        Username:   aws.String(req.Username),
        Password:   aws.String(req.Password),
        UserAttributes: attributes,
    }
    
    // CRÍTICO: Usar método privado para calcular SecretHash
    // El secret nunca se expone fuera del cliente
    if c.clientSecret != "" {
        secretHash := c.computeSecretHash(req.Username)
        input.SecretHash = aws.String(secretHash)
    }
    
    // ... resto de la implementación ...
    
    // Logging seguro: no loggear secret ni hash
    c.logger.Info(ctx, "User registered successfully",
        map[string]interface{}{
            "username": req.Username,
            "email":    req.Email,
            // NO loggear: secret, hash, password
        })
    
    return user, nil
}

// Authenticate ejemplo de uso del secret interno
func (c *Client) Authenticate(ctx context.Context, req AuthenticateRequest) (*AuthTokens, error) {
    // ... validación ...
    
    authParams := map[string]string{
        "USERNAME": req.Username,
        "PASSWORD": req.Password,
    }
    
    // CRÍTICO: Usar método privado para calcular SecretHash
    if c.clientSecret != "" {
        secretHash := c.computeSecretHash(req.Username)
        authParams["SECRET_HASH"] = secretHash
    }
    
    input := &cognitoidentityprovider.InitiateAuthInput{
        AuthFlow:     types.AuthFlowTypeUserPasswordAuth,
        ClientId:     aws.String(c.config.ClientID),
        AuthParameters: authParams,
    }
    
    // ... resto de la implementación ...
    
    // Logging seguro: no loggear password ni secret
    c.logger.Info(ctx, "User authenticated successfully",
        map[string]interface{}{
            "username": req.Username,
            // NO loggear: password, secret, tokens completos
        })
    
    return tokens, nil
}
```

---

### 3. Helpers Privados en `helpers.go`

```go
package cognito

import (
    "errors"
    "fmt"
)

// validateConfig valida la configuración
// CRÍTICO: No validar el valor del secret, solo su presencia si es necesario
func validateConfig(cfg Config) error {
    if cfg.Region == "" {
        return errors.New("region is required")
    }
    if cfg.UserPoolID == "" {
        return errors.New("user_pool_id is required")
    }
    if cfg.ClientID == "" {
        return errors.New("client_id is required")
    }
    
    // NO validar el valor del secret aquí
    // Solo validar que si ClientSecret está configurado, no esté vacío
    // (aunque esto es opcional, ya que puede ser opcional)
    
    return nil
}

// validateRegisterRequest valida el request de registro
func validateRegisterRequest(req RegisterUserRequest) error {
    if req.Username == "" {
        return ErrMissingRequiredField
    }
    if req.Email == "" {
        return ErrInvalidEmail
    }
    if req.Password == "" {
        return ErrMissingRequiredField
    }
    return nil
}

// validateAuthenticateRequest valida el request de autenticación
func validateAuthenticateRequest(req AuthenticateRequest) error {
    if req.Username == "" {
        return ErrMissingRequiredField
    }
    if req.Password == "" {
        return ErrMissingRequiredField
    }
    return nil
}
```

---

## 📝 Configuración en YAML

### `config/application.yaml`

```yaml
cognito:
  region: "${AWS_REGION}"
  user_pool_id: "${COGNITO_USER_POOL_ID}"
  client_id: "${COGNITO_CLIENT_ID}"
  
  # CRÍTICO: client_secret se carga desde variable de entorno
  # Viper resuelve ${COGNITO_CLIENT_SECRET} usando resolveEnvValue()
  # El secret nunca se almacena en el archivo YAML
  client_secret: "${COGNITO_CLIENT_SECRET}"
  
  # O con valor por defecto vacío (si la variable no existe):
  # client_secret: "${COGNITO_CLIENT_SECRET:-}"
  
  jwks_url: ""  # Auto-generado si está vacío
  token_expiration: 3600s
  
  resilience:
    circuit_breaker:
      enabled: true
      failure_threshold: 5
      timeout: 60s
    retry:
      enabled: true
      max_attempts: 3
  
  timeout: 30s
  max_retries: 3
  retry_backoff: 1s
```

---

## 🔒 Variables de Entorno

### Desarrollo Local

```bash
# .env o exportar en shell
export COGNITO_CLIENT_SECRET="tu-secret-aqui"
export COGNITO_USER_POOL_ID="us-east-1_xxxxx"
export COGNITO_CLIENT_ID="xxxxx"
export AWS_REGION="us-east-1"
```

### Producción (Docker/Kubernetes)

```yaml
# docker-compose.yml o Kubernetes Secret
environment:
  - COGNITO_CLIENT_SECRET=${COGNITO_CLIENT_SECRET}
  # O usar secrets de Kubernetes:
  # envFrom:
  #   - secretRef:
  #       name: cognito-secrets
```

---

## ✅ Checklist de Seguridad

- [x] `ClientSecret` en `Config` tiene tag `json:"-"` para evitar serialización
- [x] Secret se copia a campo privado `clientSecret` en `NewClient`
- [x] Secret se limpia de `Config` después de copiar
- [x] No hay getters públicos para el secret
- [x] `computeSecretHash` es método privado
- [x] No se loggea el secret (solo indicador `has_secret`)
- [x] No se loggean passwords ni tokens completos
- [x] Secret se carga desde variable de entorno usando `${VAR_NAME}`
- [x] Configuración YAML no contiene valores hardcodeados de secrets

---

## 🚫 Lo que NO hacer

```go
// ❌ MAL: Exponer secret públicamente
type Client struct {
    ClientSecret string // Público - NO HACER
}

// ❌ MAL: Getter público para secret
func (c *Client) GetClientSecret() string {
    return c.clientSecret // NO HACER
}

// ❌ MAL: Loggear el secret
c.logger.Info(ctx, "secret", map[string]interface{}{
    "secret": c.clientSecret, // NO HACER
})

// ❌ MAL: Serializar secret
json.Marshal(c.config) // Si ClientSecret no tiene json:"-", se serializa

// ❌ MAL: Hardcodear secret en código
clientSecret := "mi-secret-hardcodeado" // NO HACER
```

---

## ✅ Lo que SÍ hacer

```go
// ✅ BIEN: Campo privado (minúscula)
type Client struct {
    clientSecret string // Privado - CORRECTO
}

// ✅ BIEN: Método privado para calcular hash
func (c *Client) computeSecretHash(username string) string {
    // ... implementación privada
}

// ✅ BIEN: Logging seguro
c.logger.Info(ctx, "operation", map[string]interface{}{
    "has_secret": c.clientSecret != "", // Solo indicador
})

// ✅ BIEN: Cargar desde variable de entorno
client_secret: "${COGNITO_CLIENT_SECRET}" // En YAML

// ✅ BIEN: Limpiar después de copiar
cfg.ClientSecret = "" // Después de copiar a campo privado
```

---

## 📚 Referencias

- [Viper Environment Variables](https://github.com/spf13/viper#working-with-environment-variables)
- [AWS Cognito Client Secret](https://docs.aws.amazon.com/cognito/latest/developerguide/user-pool-settings-client-apps.html)
- [OWASP Secret Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)

---

**Última actualización:** 2026-01-06  
**Versión:** 1.0
