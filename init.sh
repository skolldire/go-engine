#!/bin/bash

REPO_NAME=$(basename "$(git rev-parse --show-toplevel 2>/dev/null || pwd)")
MOD_FILE="go.mod"
OLD_MODULE_PATH="github.com/skolldire/go-kit-library"
NEW_MODULE_PATH="github.com/skolldire/$REPO_NAME"

if [ -f "$MOD_FILE" ]; then
    CURRENT_NAME=$(grep "^module" "$MOD_FILE" | awk '{print $2}')
    echo "Actualizando el nombre del módulo en $MOD_FILE..."
    go mod edit -module="$NEW_MODULE_PATH"
    echo "Nombre del módulo actualizado de $CURRENT_NAME a $NEW_MODULE_PATH"
else
    echo "No se encontró $MOD_FILE. Creando uno nuevo..."
    go mod init "$NEW_MODULE_PATH"
fi

echo "Actualizando las rutas de importación de '$OLD_MODULE_PATH' a '$NEW_MODULE_PATH'..."
find . -type f -name "*.go" -exec sed -i '' "s|$OLD_MODULE_PATH|$NEW_MODULE_PATH|g" {} \; 2>/dev/null ||
    find . -type f -name "*.go" -exec sed -i "s|$OLD_MODULE_PATH|$NEW_MODULE_PATH|g" {} \;
echo "Actualización de rutas de importación completada."

# El repositorio son varios módulos: ordenarlos uno a uno y luego sincronizar el
# workspace es lo unico que deja el checkout consistente.
echo "Ejecutando 'go mod tidy' en cada módulo..."
for m in . aws messaging http database/sql database/redis database/mongodb database/memcached preset/full; do
    echo "  -> $m"
    (cd "$m" && GOWORK=off go mod tidy)
done
go work sync
echo "'go mod tidy' completado."
