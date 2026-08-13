package main

import (
	"fmt"
	"os"

	"lopiibot.com/internal/config"
	"lopiibot.com/internal/server"
)

// El error se IMPRIME antes de salir. Antes los dos caminos hacían os.Exit(1)
// mudo: un arranque fallido —config mal, Postgres caído, token vacío, webhook
// rechazado— se veía igual que cualquier otro, y había que bisectar a mano para
// saber cuál de los cuatro era. El logger todavía no existe acá (lo instala
// InitServer), así que va a stderr crudo.
func main() {
	cfg, err := config.Initialize()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	if err := server.InitServer(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
