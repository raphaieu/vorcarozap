package static

import "embed"

// FS contém os arquivos estáticos (CSS, etc.) embutidos no binário.
//
//go:embed css/*
var FS embed.FS
