package migrations

import "embed"

// FS contém os arquivos de migrations SQL embutidos no binário.
//
//go:embed *.sql
var FS embed.FS
