// Command portwhy explains why a local port is taken and offers to free it.
package main

import (
	"os"

	"github.com/JtheGunner/portwhy/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
