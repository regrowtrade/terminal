# Despliegue objetivo

No se ha desplegado ningún servicio ni creado infraestructura con esta entrega.

## Recomendación

- Next.js en Vercel, con `apps/web` como directorio de la aplicación dentro del repositorio terminal y el lockfile de workspace disponible.
- Motor Go en un VPS de DigitalOcean, cerca de la base de datos, con IP de salida estable y proceso siempre activo.
- Neon PostgreSQL para persistencia una vez integrado; región alineada con el motor.

Elegir región según disponibilidad permitida de Binance/Bybit y latencia medida. No usar la infraestructura para eludir restricciones de los exchanges.

## Primera entrega

El motor puede compilarse con `infra/engine.Dockerfile` desde el contexto raíz de terminal. Escucha en `0.0.0.0:8080` solo dentro del contenedor. Publicar en loopback o red privada; no exponer el puerto sin protección.

Para comunicar Vercel y el VPS se requiere HTTPS válido mediante un proxy (Caddy/Nginx), acceso restringido cuando sea posible y ENGINE_API_TOKEN único. El frontend usa ese token en el servidor, no en el navegador.

Configurar en la web:

- TERMINAL_PASSWORD: contraseña personal única, temporal para esta fase.
- SESSION_SECRET: secreto aleatorio de 32+ caracteres.
- APP_ORIGIN: origen HTTPS exacto del dominio (sin path).
- ENGINE_URL: origen HTTPS del motor, sin path/query/credenciales.
- ENGINE_API_TOKEN: igual al configurado en el motor.

No escalar públicamente este login: sustituirlo por identidad/MFA y limitación compartida antes de abrir registros. Configurar alertas, backups y despliegues sin concurrencia de ejecutores antes de activar trading.

Las plantillas no contienen credenciales. Los secretos deben inyectarse mediante el gestor del proveedor, nunca durante la construcción de imágenes ni en el repositorio.
