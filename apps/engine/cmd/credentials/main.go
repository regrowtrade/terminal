// Administrative credential lifecycle. Never run with the HTTP service's role.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
)

func readHidden(label string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("se requiere una terminal interactiva con entrada oculta; no usar argumentos, pipes ni archivos de claves")
	}
	fmt.Fprint(os.Stderr, label+": ")
	data, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	defer clear(data)
	if err != nil {
		return "", errors.New("no se pudo leer la entrada oculta")
	}
	return string(data), nil
}
func run() error {
	action := flag.String("action", "", "provision, rotate (cifrado) o delete (solo copia local)")
	account := flag.String("account", "", "UUID de cuenta para rotate/delete")
	name := flag.String("name", "Binance Futures USDT · Lectura", "nombre de la cuenta")
	flag.Parse()
	if flag.NArg() != 0 || (*action != "provision" && *action != "rotate" && *action != "delete") {
		return errors.New("indica una acción válida; nunca pases credenciales como argumentos")
	}
	if (*action == "provision" && *account != "") || (*action != "provision" && *account == "") {
		return errors.New("provision crea una cuenta nueva; rotate/delete requieren su UUID")
	}
	uri := os.Getenv("MIGRATION_DATABASE_URL")
	if uri == "" {
		return errors.New("configura la URL administrativa solo en esta consola")
	}
	var keys *vault.Keyring
	var credentials vault.Credentials
	var err error
	if *action != "delete" {
		keys, err = vault.New(os.Getenv("CREDENTIAL_ACTIVE_KEY_ID"), os.Getenv("CREDENTIAL_KEYS_JSON"))
		if err != nil {
			return vault.ErrVault
		}
	}
	if *action == "provision" {
		if os.Getenv("BINANCE_READ_ENABLED") != "true" || os.Getenv("BINANCE_EGRESS_CONFIRMED") != "true" {
			return errors.New("lectura desactivada; aprobar región e IP de salida fija restringida antes de introducir claves")
		}
		credentials.APIKey, err = readHidden("API key HMAC (lectura, sin trading/retiros/transferencias)")
		if err != nil {
			return err
		}
		credentials.APISecret, err = readHidden("API secret HMAC")
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if *action == "provision" {
		// Validate permissions and actual Futures read access, without printing
		// or persisting the snapshot. There are no exchange writes or retries.
		if _, err := binance.New().Snapshot(ctx, credentials); err != nil {
			return errors.New("no se pudo verificar lectura segura en Binance; no se guardaron credenciales")
		}
	}
	id, err := database.ManageCredential(ctx, uri, *action, *account, *name, keys, credentials)
	if err != nil {
		return errors.New("no se pudo confirmar el cambio; revisar configuración, permisos y estado antes de reintentar")
	}
	fmt.Fprintln(os.Stdout, "Cambio administrativo confirmado. Cuenta:", id)
	if *action == "delete" {
		fmt.Fprintln(os.Stdout, "Solo se eliminó la copia cifrada local. Revoca también la clave en Binance; no se cancelaron órdenes ni se cerraron posiciones.")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
