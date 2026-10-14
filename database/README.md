# Persistencia futura

`0001_foundation.sql` es una base de diseño para PostgreSQL, no una migración aplicada por el runtime. No ejecutar sobre la base del journal antiguo: crear una base o esquema aislado y revisar la migración primero.

Incluye propietarios, FKs compuestas para evitar vincular recursos de usuarios/cuentas diferentes, IDs de intención deduplicables, fills, ledger de funding, notas y RLS. La aplicación deberá usar un rol sin BYPASSRLS y establecer `app.user_id` con `SET LOCAL` dentro de cada transacción a partir de identidad verificada.

Falta integrar el driver, repositorios, migrador versionado, pruebas de aislamiento reales en PostgreSQL y política de grants. No se afirma que RLS esté activo en una instalación actual. Las credenciales se referencian a un almacén cifrado externo aún no implementado; no hay columnas de claves en texto plano.

`numeric(38,18)` requiere una decisión explícita de cuantización según cada instrumento antes de persistir valores calculados de 32 decimales. Las ejecuciones históricas deben conservar sus cantidades exactas y moneda original; rebates negativos de comisiones requerirán ampliar el modelo inicial antes de un adaptador que los soporte.
