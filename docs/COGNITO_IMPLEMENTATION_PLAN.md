# Plan de Implementación - Cliente AWS Cognito para go-engine

## 📋 Resumen Ejecutivo

Este documento detalla el plan de implementación para integrar un cliente AWS Cognito robusto y reutilizable en el framework go-engine, siguiendo los mismos patrones y principios de diseño que los demás clientes del framework.

**Prioridad:** 🔴 ALTA - Bloquea MVP 0  
**Estimación Total:** 3-4 semanas  
**Complejidad:** Media-Alta

---

## 🎯 Objetivos

1. ✅ Implementar cliente Cognito siguiendo patrones de go-engine
2. ✅ Integrar con el sistema de configuración de go-engine
3. ✅ Implementar funcionalidades críticas para MVP 0
4. ✅ Asegurar robustez (resilience, logging, error handling)
5. ✅ Validación de tokens JWT usando JWKS de Cognito
6. ✅ Soporte para MFA (SMS y TOTP)

---

## 📁 Estructura de Archivos

```
pkg/clients/cognito/
├── entity.go              # Config, Entities, Errors, Interfaces
├── service.go             # Implementación del servicio
├── service_test.go        # Tests unitarios
├── jwks.go                # Cliente JWKS para validación de tokens
├── jwks_test.go           # Tests de JWKS
├── helpers.go             # Funciones auxiliares (secret hash, validaciones)
├── helpers_test.go        # Tests de helpers
└── README.md              # Documentación de uso
```

---

## 🗓️ Fases de Implementación

### **FASE 0: Setup y Preparación** (2-3 días)

**Objetivo:** Preparar el entorno y estructura base

#### Tareas:
- [ ] **T0.1:** Crear estructura de directorios `pkg/clients/cognito/`
- [ ] **T0.2:** Agregar dependencias al `go.mod`:
  ```go
  github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider v1.41.0
  github.com/golang-jwt/jwt/v5 v5.2.0
  github.com/lestrrat-go/jwx/v2 v2.0.0  // Para JWKS
  ```
- [ ] **T0.3:** Crear archivo `entity.go` con estructuras base:
  - `Config` struct
  - `Service` interface (vacía inicialmente)
  - `Client` struct (vacío inicialmente)
  - Errores básicos
- [ ] **T0.4:** Crear archivo `service.go` con función `NewClient` básica
- [ ] **T0.5:** Crear archivo `README.md` con estructura básica

**Criterios de Aceptación:**
- ✅ Estructura de archivos creada
- ✅ Dependencias agregadas y `go mod tidy` ejecutado sin errores
- ✅ Compilación exitosa con estructura base

---

### **FASE 1: MVP 0 - Core Essential** (1 semana)

**Prioridad:** 🔴 CRÍTICA - Bloquea MVP 0

#### **Sprint 1.1: Configuración y Estructuras Base** (2 días)

- [ ] **T1.1.1:** Completar `entity.go`:
  - [ ] `Config` struct completo con todos los campos
    - [ ] `ClientSecret` con tag `json:"-"` para evitar serialización
    - [ ] Documentar que `client_secret` debe cargarse desde `${VAR_NAME}` en YAML
  - [ ] `User` struct con todos los campos
  - [ ] `AuthTokens` struct
  - [ ] `TokenClaims` struct
  - [ ] Request structs: `RegisterUserRequest`, `AuthenticateRequest`, `ConfirmSignUpRequest`
  - [ ] Todos los errores definidos
  - [ ] `CognitoError` struct con métodos `Error()` y `Unwrap()`
  - [ ] Constantes y tipos (`UserStatus`, `MFAChallengeType`)

- [ ] **T1.1.2:** Implementar `NewClient` en `service.go`:
  - [ ] Validación de configuración (`validateConfig`)
  - [ ] **CRÍTICO:** Manejo seguro de `client_secret`:
    - [ ] Copiar `ClientSecret` de `Config` a campo privado del cliente
    - [ ] Limpiar `ClientSecret` de `Config` después de copiar (opcional pero recomendado)
    - [ ] Validar que secret se carga correctamente desde variable de entorno
  - [ ] Creación de cliente AWS SDK v2
  - [ ] Configuración de JWKS URL (auto-generado si está vacío)
  - [ ] Inicialización de cliente JWKS
  - [ ] Inicialización de servicio de resiliencia
  - [ ] Logging de inicialización (sin exponer secret, solo indicar si está presente)

- [ ] **T1.1.3:** Crear `helpers.go`:
  - [ ] `validateConfig(cfg Config) error`
  - [ ] `computeSecretHash(clientID, clientSecret, username) string`
    - [ ] **CRÍTICO:** Esta función debe ser privada y solo usarse internamente
    - [ ] No debe exponerse el secret ni el hash calculado
  - [ ] `validateRegisterRequest(req RegisterUserRequest) error`
  - [ ] `validateAuthenticateRequest(req AuthenticateRequest) error`
  - [ ] Funciones auxiliares para extraer claims de JWT

- [ ] **T1.1.4:** Crear `jwks.go`:
  - [ ] Cliente JWKS básico para obtener claves públicas de Cognito
  - [ ] Cache de claves JWKS (opcional pero recomendado)
  - [ ] Función para obtener clave pública por `kid`

**Criterios de Aceptación:**
- ✅ Todas las estructuras definidas y documentadas
- ✅ `NewClient` crea instancia válida sin errores
- ✅ Validaciones funcionan correctamente
- ✅ JWKS client puede obtener claves de Cognito

---

#### **Sprint 1.2: Registro y Confirmación** (1.5 días)

- [ ] **T1.2.1:** Implementar `RegisterUser`:
  - [ ] Validación de request
  - [ ] Preparación de atributos de usuario
  - [ ] Llamada a `SignUp` de AWS SDK
  - [ ] Manejo de `SecretHash` si `ClientSecret` está configurado:
    - [ ] **CRÍTICO:** Usar método privado `computeSecretHash` con secret interno
    - [ ] No exponer secret ni hash en logs
  - [ ] Conversión de respuesta AWS a `User`
  - [ ] Manejo de errores con `handleError`
  - [ ] Logging estructurado (sin información sensible)
  - [ ] Integración con resiliencia

- [ ] **T1.2.2:** Implementar `ConfirmSignUp`:
  - [ ] Validación de request
  - [ ] Llamada a `ConfirmSignUp` de AWS SDK
  - [ ] Manejo de errores (código inválido, expirado, etc.)
  - [ ] Logging estructurado
  - [ ] Integración con resiliencia

- [ ] **T1.2.3:** Implementar `handleError`:
  - [ ] Mapeo de errores AWS SDK a errores tipados
  - [ ] Manejo de `InvalidParameterException`
  - [ ] Manejo de `ResourceNotFoundException`
  - [ ] Manejo de `NotAuthorizedException`
  - [ ] Manejo de `LimitExceededException`
  - [ ] Manejo de `TooManyRequestsException`
  - [ ] Manejo de `CodeMismatchException`
  - [ ] Manejo de `ExpiredCodeException`
  - [ ] Manejo de `UsernameExistsException`
  - [ ] Manejo de `UserNotFoundException`

- [ ] **T1.2.4:** Tests unitarios para `RegisterUser` y `ConfirmSignUp`:
  - [ ] Test de registro exitoso
  - [ ] Test de registro con usuario existente
  - [ ] Test de registro con contraseña inválida (password policy)
  - [ ] Test de confirmación exitosa
  - [ ] Test de confirmación con código inválido
  - [ ] Test de confirmación con código expirado

**Criterios de Aceptación:**
- ✅ `RegisterUser` funciona correctamente
- ✅ `ConfirmSignUp` funciona correctamente
- ✅ Errores se mapean correctamente
- ✅ Tests unitarios pasan con >80% cobertura

---

#### **Sprint 1.3: Autenticación y Tokens** (2 días)

- [ ] **T1.3.1:** Implementar `Authenticate`:
  - [ ] Validación de request
  - [ ] Llamada a `InitiateAuth` con `AuthFlowTypeUserPasswordAuth`
  - [ ] Manejo de `SecretHash` si está configurado:
    - [ ] **CRÍTICO:** Usar método privado `computeSecretHash` con secret interno
    - [ ] No exponer secret ni hash en logs
  - [ ] Extracción de tokens de respuesta
  - [ ] Manejo de caso MFA requerido (retornar error especial con `SessionToken`)
  - [ ] Conversión a `AuthTokens`
  - [ ] Logging estructurado (sin passwords ni tokens completos)
  - [ ] Integración con resiliencia

- [ ] **T1.3.2:** Implementar `ValidateToken`:
  - [ ] Parsear token JWT
  - [ ] Validar firma usando JWKS de Cognito
  - [ ] Obtener clave pública por `kid` del header
  - [ ] Validar algoritmo de firma (debe ser RSA)
  - [ ] Validar `issuer` (debe contener `UserPoolID`)
  - [ ] Validar `audience` (debe ser `ClientID`)
  - [ ] Validar expiración (`exp`)
  - [ ] Extraer claims del token
  - [ ] Convertir a `TokenClaims`
  - [ ] Cache de claves JWKS para performance

- [ ] **T1.3.3:** Implementar `GetUserByAccessToken`:
  - [ ] Llamada a `GetUser` de AWS SDK con `AccessToken`
  - [ ] Conversión de respuesta AWS a `User`
  - [ ] Manejo de errores (token inválido, expirado)
  - [ ] Logging estructurado
  - [ ] Integración con resiliencia

- [ ] **T1.3.4:** Completar `jwks.go`:
  - [ ] Implementar cache de claves JWKS
  - [ ] Implementar refresh de cache cuando expire
  - [ ] Manejo de errores de red al obtener JWKS

- [ ] **T1.3.5:** Tests unitarios:
  - [ ] Test de autenticación exitosa (sin MFA)
  - [ ] Test de autenticación con MFA requerido
  - [ ] Test de autenticación con credenciales inválidas
  - [ ] Test de validación de token válido
  - [ ] Test de validación de token inválido
  - [ ] Test de validación de token expirado
  - [ ] Test de validación de token con firma inválida
  - [ ] Test de `GetUserByAccessToken` exitoso
  - [ ] Test de `GetUserByAccessToken` con token inválido

**Criterios de Aceptación:**
- ✅ `Authenticate` funciona correctamente (con y sin MFA)
- ✅ `ValidateToken` valida tokens correctamente usando JWKS
- ✅ `GetUserByAccessToken` funciona correctamente
- ✅ Cache de JWKS funciona correctamente
- ✅ Tests unitarios pasan con >85% cobertura

---

### **FASE 2: MVP 0 - MFA Support** (2-3 días)

**Prioridad:** 🟡 MEDIA-ALTA - Necesario si MFA está habilitado

- [ ] **T2.1:** Implementar `RespondToMFAChallenge`:
  - [ ] Validación de request (session token, código MFA, tipo de challenge)
  - [ ] Llamada a `RespondToAuthChallenge` de AWS SDK
  - [ ] Manejo de `SMS_MFA` challenge
  - [ ] Manejo de `SOFTWARE_TOKEN_MFA` challenge
  - [ ] Extracción de tokens después de MFA exitoso
  - [ ] Manejo de errores (código inválido, expirado)
  - [ ] Logging estructurado
  - [ ] Integración con resiliencia

- [ ] **T2.2:** Mejorar `handleError` para errores MFA:
  - [ ] Manejo de `CodeMismatchException` en contexto MFA
  - [ ] Manejo de `ExpiredCodeException` en contexto MFA
  - [ ] Retornar `SessionToken` en error cuando MFA es requerido

- [ ] **T2.3:** Actualizar `Authenticate` para manejar respuesta MFA:
  - [ ] Detectar cuando Cognito retorna `ChallengeName`
  - [ ] Extraer `SessionToken` de la respuesta
  - [ ] Retornar error especial `MFARequired` con `SessionToken`

- [ ] **T2.4:** Tests unitarios:
  - [ ] Test de `RespondToMFAChallenge` con SMS exitoso
  - [ ] Test de `RespondToMFAChallenge` con TOTP exitoso
  - [ ] Test de `RespondToMFAChallenge` con código inválido
  - [ ] Test de `RespondToMFAChallenge` con código expirado
  - [ ] Test de flujo completo: Authenticate → MFA → Tokens

**Criterios de Aceptación:**
- ✅ `RespondToMFAChallenge` funciona para SMS y TOTP
- ✅ Flujo completo de autenticación con MFA funciona
- ✅ Errores MFA se manejan correctamente
- ✅ Tests unitarios pasan con >80% cobertura

---

### **FASE 3: MVP 1 - Funcionalidades Adicionales** (3-4 días)

**Prioridad:** 🟢 MEDIA - Mejora UX pero no bloquea

- [ ] **T3.1:** Implementar `RefreshToken`:
  - [ ] Validación de refresh token
  - [ ] Llamada a `InitiateAuth` con `AuthFlowTypeRefreshTokenAuth`
  - [ ] Extracción de nuevos tokens
  - [ ] Manejo de errores (token inválido, expirado)
  - [ ] Logging estructurado
  - [ ] Integración con resiliencia

- [ ] **T3.2:** Implementar `ForgotPassword`:
  - [ ] Validación de request
  - [ ] Llamada a `ForgotPassword` de AWS SDK
  - [ ] Manejo de errores (usuario no encontrado, etc.)
  - [ ] Logging estructurado
  - [ ] Integración con resiliencia

- [ ] **T3.3:** Implementar `ConfirmForgotPassword`:
  - [ ] Validación de request
  - [ ] Llamada a `ConfirmForgotPassword` de AWS SDK
  - [ ] Validación de nueva contraseña (password policy)
  - [ ] Manejo de errores (código inválido, contraseña inválida)
  - [ ] Logging estructurado
  - [ ] Integración con resiliencia

- [ ] **T3.4:** Tests unitarios:
  - [ ] Test de `RefreshToken` exitoso
  - [ ] Test de `RefreshToken` con token inválido
  - [ ] Test de `ForgotPassword` exitoso
  - [ ] Test de `ConfirmForgotPassword` exitoso
  - [ ] Test de `ConfirmForgotPassword` con contraseña inválida

**Criterios de Aceptación:**
- ✅ Todas las funcionalidades MVP 1 implementadas
- ✅ Tests unitarios pasan con >80% cobertura
- ✅ Documentación actualizada

---

### **FASE 4: Integración con go-engine** (2-3 días)

**Prioridad:** 🔴 CRÍTICA - Necesario para usar en aplicaciones

#### **Sprint 4.1: Integración con Configuración** (1 día)

- [ ] **T4.1.1:** Agregar `Cognito` a `pkg/config/viper/entity.go`:
  ```go
  Cognito *cognito.Config `mapstructure:"cognito"`
  ```

- [ ] **T4.1.2:** Agregar import de cognito en `pkg/config/viper/entity.go`

- [ ] **T4.1.3:** Crear ejemplo de configuración en documentación:
  ```yaml
  cognito:
    region: "${AWS_REGION}"
    user_pool_id: "${COGNITO_USER_POOL_ID}"
    client_id: "${COGNITO_CLIENT_ID}"
    # CRÍTICO: client_secret debe cargarse desde variable de entorno
    # El secret se encapsula en el cliente y no se expone
    client_secret: "${COGNITO_CLIENT_SECRET}"  # Opcional, pero recomendado para seguridad
    # O con valor por defecto vacío:
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
  
  **Nota de Seguridad:** 
  - El `client_secret` se carga desde la variable de entorno `COGNITO_CLIENT_SECRET`
  - Una vez cargado, se almacena de forma privada en el cliente
  - No hay acceso público al secret después de la inicialización
  - No se loggea ni se serializa el secret

**Criterios de Aceptación:**
- ✅ Configuración se carga correctamente desde YAML
- ✅ Variables de entorno se resuelven correctamente

---

#### **Sprint 4.2: Integración con App Builder** (1 día)

- [ ] **T4.2.1:** Agregar campo `CognitoClient` a `pkg/app/entity.go`:
  ```go
  CognitoClient cognito.Service
  ```

- [ ] **T4.2.2:** Agregar método `GetCognito()` a `pkg/app/entity.go`:
  ```go
  func (e *Engine) GetCognito() cognito.Service {
      return e.CognitoClient
  }
  ```

- [ ] **T4.2.3:** Implementar `createClientCognito` en `pkg/app/service.go`:
  ```go
  func (i *clients) createClientCognito(cfg *cognito.Config) cognito.Service {
      if cfg == nil {
          return nil
      }
      client, err := cognito.NewClient(*cfg, i.log)
      if err != nil {
          i.setError(err)
          return nil
      }
      return client
  }
  ```

- [ ] **T4.2.4:** Llamar a `createClientCognito` en `Init()` de `pkg/app/service.go`:
  ```go
  c.Engine.CognitoClient = initializer.createClientCognito(c.Engine.Conf.Cognito)
  ```

- [ ] **T4.2.5:** Agregar import de cognito en `pkg/app/service.go`

**Criterios de Aceptación:**
- ✅ Cognito se inicializa automáticamente si está configurado
- ✅ Errores de inicialización se capturan correctamente
- ✅ `GetCognito()` retorna el cliente correctamente

---

#### **Sprint 4.3: Tests de Integración** (1 día)

- [ ] **T4.3.1:** Crear test de integración en `pkg/app/service_test.go`:
  - [ ] Test de inicialización de Cognito desde configuración
  - [ ] Test de inicialización sin configuración (debe ser nil)
  - [ ] Test de manejo de errores de inicialización

- [ ] **T4.3.2:** Crear ejemplo completo en documentación:
  ```go
  engine, err := app.NewAppBuilder().
      WithContext(ctx).
      WithConfigs().
      WithInitialization().
      WithRouter().
      Build()
  
  cognitoClient := engine.GetCognito()
  if cognitoClient != nil {
      // Usar Cognito
  }
  ```

**Criterios de Aceptación:**
- ✅ Tests de integración pasan
- ✅ Ejemplo funciona correctamente
- ✅ Documentación completa

---

### **FASE 5: Documentación y Ejemplos** (2 días)

- [ ] **T5.1:** Completar `pkg/clients/cognito/README.md`:
  - [ ] Descripción general
  - [ ] Instalación y configuración
  - [ ] Ejemplos de uso para cada método
  - [ ] Manejo de errores
  - [ ] Flujos de autenticación (con y sin MFA)
  - [ ] Validación de tokens
  - [ ] Troubleshooting

- [ ] **T5.2:** Crear ejemplos en `examples/`:
  - [ ] `examples/cognito_basic.go` - Registro y autenticación básica
  - [ ] `examples/cognito_mfa.go` - Autenticación con MFA
  - [ ] `examples/cognito_middleware.go` - Middleware de validación de tokens
  - [ ] `examples/cognito_refresh.go` - Renovación de tokens

- [ ] **T5.3:** Actualizar documentación principal de go-engine:
  - [ ] Agregar sección de Cognito en README principal
  - [ ] Actualizar lista de clientes disponibles

**Criterios de Aceptación:**
- ✅ Documentación completa y clara
- ✅ Ejemplos funcionan correctamente
- ✅ Documentación principal actualizada

---

### **FASE 6: Testing y Calidad** (2-3 días)

- [ ] **T6.1:** Tests de integración con Cognito real:
  - [ ] Setup de entorno de pruebas (Cognito User Pool de prueba)
  - [ ] Test de registro completo
  - [ ] Test de autenticación completo
  - [ ] Test de MFA completo
  - [ ] Test de validación de tokens
  - [ ] Test de refresh token
  - [ ] Test de forgot password

- [ ] **T6.2:** Tests de performance:
  - [ ] Benchmark de `ValidateToken` (con cache de JWKS)
  - [ ] Benchmark de `Authenticate`
  - [ ] Benchmark de `RegisterUser`

- [ ] **T6.3:** Tests de resiliencia:
  - [ ] Test de circuit breaker
  - [ ] Test de retry automático
  - [ ] Test de timeout

- [ ] **T6.4:** Revisión de código:
  - [ ] Code review completo
  - [ ] Verificar que sigue patrones de go-engine
  - [ ] Verificar manejo de errores
  - [ ] Verificar logging estructurado
  - [ ] Verificar que no hay información sensible en logs

- [ ] **T6.5:** Linting y formateo:
  - [ ] Ejecutar `golangci-lint` y corregir errores
  - [ ] Ejecutar `gofmt` y verificar formato
  - [ ] Verificar que no hay warnings

**Criterios de Aceptación:**
- ✅ Cobertura de tests >85%
- ✅ Todos los tests pasan
- ✅ Performance dentro de objetivos (<1s RegisterUser, <500ms Authenticate, <50ms ValidateToken)
- ✅ Code review aprobado
- ✅ Linting sin errores

---

## 📦 Dependencias Requeridas

### Nuevas Dependencias:
```go
require (
    github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider v1.41.0
    github.com/golang-jwt/jwt/v5 v5.2.0
    github.com/lestrrat-go/jwx/v2 v2.0.0  // Para JWKS
)
```

### Dependencias Existentes (ya en go-engine):
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/config`
- `github.com/skolldire/go-engine/pkg/utilities/logger`
- `github.com/skolldire/go-engine/pkg/utilities/resilience`

---

## 🔍 Consideraciones Técnicas

### 1. Manejo Seguro de Secrets (CRÍTICO)
- **Importante:** `client_secret` debe cargarse desde variables de entorno usando Viper
- **Encapsulación:** El secret se almacena en el cliente pero NO se expone públicamente
- **Configuración:** Usar sintaxis `${VAR_NAME}` o `${VAR_NAME:-default}` en YAML
- **Seguridad:**
  - El campo `ClientSecret` en `Config` debe ser `string` pero marcado como no exportado en la struct interna del cliente
  - Una vez cargado, el secret se almacena en el cliente de forma privada
  - No debe haber métodos getter para el secret
  - No debe loggearse el secret (solo indicar si está presente o no)
- **Ejemplo de configuración:**
  ```yaml
  cognito:
    client_secret: "${COGNITO_CLIENT_SECRET}"  # Carga desde variable de entorno
    # O con valor por defecto:
    # client_secret: "${COGNITO_CLIENT_SECRET:-}"
  ```
- **Implementación:**
  ```go
  // En entity.go - Config es público para Viper
  type Config struct {
      ClientSecret string `mapstructure:"client_secret" json:"-"` // json:"-" evita serialización
  }
  
  // En service.go - Cliente interno con secret privado
  type Client struct {
      config        Config  // Config completo (incluye secret)
      clientSecret  string  // Campo privado, no exportado
      // ... otros campos
  }
  
  // En NewClient - Cargar secret y limpiar de Config si es necesario
  func NewClient(cfg Config, log logger.Service) (Service, error) {
      client := &Client{
          config: cfg,
          clientSecret: cfg.ClientSecret, // Copiar a campo privado
      }
      // Opcional: Limpiar secret de config para evitar exposición accidental
      cfg.ClientSecret = ""
      return client, nil
  }
  ```

### 2. Validación de Tokens JWT
- **Importante:** Cognito genera y firma los tokens automáticamente
- Este cliente solo valida la firma usando las claves públicas de Cognito (JWKS)
- Las claves JWKS deben cachearse para performance
- El endpoint JWKS es: `https://cognito-idp.{region}.amazonaws.com/{userPoolId}/.well-known/jwks.json`

### 3. MFA (Multi-Factor Authentication)
- **Importante:** MFA se configura en AWS Cognito Console, no en este cliente
- El cliente solo consume el flujo MFA configurado en Cognito
- Soporta SMS MFA y TOTP/Software Token MFA
- Si MFA está activado, `Authenticate` retorna error especial con `SessionToken`
- Debe llamarse `RespondToMFAChallenge` para completar autenticación

### 4. Password Policy
- **Importante:** Password policy se configura en AWS Cognito Console
- Este cliente respeta las políticas configuradas en Cognito
- Si una contraseña no cumple, Cognito retorna error que este cliente propaga

### 5. Secret Hash
- Si `ClientSecret` está configurado, debe calcularse `SecretHash` para ciertas operaciones
- Fórmula: `HMAC_SHA256(clientSecret, username + clientID)`
- Base64 encode del resultado
- **Importante:** El cálculo de `SecretHash` debe hacerse internamente usando el secret privado
- No debe exponerse el secret ni el hash en logs o respuestas

### 6. Resiliencia
- Usar `resilience.Service` de go-engine para circuit breaker y retry
- Configurar timeouts apropiados
- Manejar rate limiting de Cognito

### 7. Logging
- Usar `logger.Service` de go-engine
- **CRÍTICO:** No loggear información sensible:
  - ❌ NO loggear `client_secret`
  - ❌ NO loggear passwords
  - ❌ NO loggear tokens completos (solo indicar presencia)
  - ✅ Loggear operaciones importantes con contexto estructurado
  - ✅ Loggear si `client_secret` está presente o no (sin valor)

---

## ✅ Criterios de Aceptación Finales

### Funcionalidad
- [ ] Todas las operaciones MVP 0 funcionan correctamente
- [ ] Manejo de errores robusto y tipado
- [ ] Validación de tokens correcta usando JWKS
- [ ] MFA funciona correctamente (SMS y TOTP)
- [ ] Integración con go-engine funciona correctamente

### Performance
- [ ] `RegisterUser` < 1s
- [ ] `Authenticate` < 500ms
- [ ] `ValidateToken` < 50ms (con cache de JWKS)

### Seguridad
- [ ] Tokens JWT validados correctamente usando JWKS de Cognito
- [ ] **CRÍTICO:** No exposición de información sensible en logs:
  - [ ] `client_secret` no se loggea (solo presencia)
  - [ ] Passwords no se loggean
  - [ ] Tokens completos no se loggean (solo indicadores)
- [ ] **CRÍTICO:** Manejo seguro de secretos:
  - [ ] `client_secret` se carga desde variable de entorno usando `${VAR_NAME}`
  - [ ] Secret se encapsula en cliente (campo privado, no exportado)
  - [ ] No hay getters públicos para el secret
  - [ ] Secret no se serializa en JSON (`json:"-"` tag)
  - [ ] Secret se limpia de Config después de copiar al cliente
- [ ] MFA funciona correctamente si está habilitado
- [ ] Password policy de Cognito se respeta

### Observabilidad
- [ ] Logging de todas las operaciones importantes
- [ ] Traces distribuidos funcionando (si está configurado)
- [ ] Métricas expuestas (si está configurado)

### Documentación
- [ ] README.md completo con guía de uso
- [ ] Ejemplos de código funcionando
- [ ] Documentación de configuración
- [ ] Troubleshooting guide

### Calidad
- [ ] Cobertura de tests >85%
- [ ] Code review aprobado
- [ ] Linting sin errores
- [ ] Tests de integración pasando

---

## 🚀 Orden de Implementación Recomendado

1. **Fase 0** → Setup básico
2. **Fase 1.1** → Estructuras y configuración
3. **Fase 1.2** → Registro y confirmación
4. **Fase 1.3** → Autenticación y tokens
5. **Fase 2** → MFA support
6. **Fase 4** → Integración con go-engine (puede hacerse en paralelo con Fase 3)
7. **Fase 3** → Funcionalidades adicionales (MVP 1)
8. **Fase 5** → Documentación
9. **Fase 6** → Testing y calidad

---

## 📝 Notas Adicionales

### Principio YAGNI
- Solo implementar métodos esenciales para MVP 0 y MVP 1
- Métodos adicionales (gestión de usuarios, grupos, etc.) se agregan cuando sean realmente necesarios
- No implementar "por si acaso"

### Compatibilidad
- Seguir los mismos patrones que otros clientes de go-engine (SQS, SNS, etc.)
- Mantener compatibilidad con el sistema de configuración existente
- No romper APIs existentes

### Testing
- Priorizar tests unitarios para métodos críticos
- Tests de integración con Cognito real (usar User Pool de prueba)
- Mockear AWS SDK para tests unitarios

---

## 🎯 Métricas de Éxito

- ✅ Cliente Cognito funcional y listo para producción
- ✅ Integrado correctamente con go-engine
- ✅ Documentación completa y clara
- ✅ Tests con >85% cobertura
- ✅ Performance dentro de objetivos
- ✅ Code review aprobado
- ✅ Listo para usar en MVP 0

---

**Última actualización:** 2026-01-06  
**Versión del Plan:** 1.0  
**Propietario:** go-engine Team
