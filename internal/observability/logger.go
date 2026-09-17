package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/raphaieu/vorcarozap/internal/config"
)

var (
	bearerRegex     = regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9_\-\.]+`)
	openRouterRegex = regexp.MustCompile(`sk-or-v1-[a-zA-Z0-9]+`)
	emailRegex      = regexp.MustCompile(`(?i)[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRegex      = regexp.MustCompile(`(\+?[0-9]{1,3}[-.\s]?)?(\(?[0-9]{2,3}\)?[-.\s]?)?[0-9]{4,5}[-.\s]?[0-9]{4}`)
)

var sensitiveKeySubstrings = []string{
	"api_key",
	"apikey",
	"secret",
	"password",
	"hash",
	"token",
	"mfa_key",
	"encryption_key",
	"totp",
	"auth",
	"authorization",
	"cookie",
	"session_id",
	"access_key",
	"secret_key",
	"contact_email",
	"contact_phone",
	"bearer",
}

// isSensitiveKey verifica se a chave do atributo remete a dados confidenciais.
func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, sub := range sensitiveKeySubstrings {
		if strings.Contains(normalized, sub) {
			return true
		}
	}
	return false
}

// SanitizeStringValue remove padrões de segredos, emails e tokens de strings.
func SanitizeStringValue(val string) string {
	if val == "" {
		return val
	}
	res := bearerRegex.ReplaceAllString(val, "Bearer [REDACTED]")
	res = openRouterRegex.ReplaceAllString(res, "sk-or-v1-[REDACTED]")
	res = emailRegex.ReplaceAllString(res, "[REDACTED_EMAIL]")
	// Não aplicar regex de telefone se for formato de data/hora RFC3339
	if !strings.Contains(res, "T") && !strings.Contains(res, "Z") {
		res = phoneRegex.ReplaceAllString(res, "[REDACTED_PHONE]")
	}
	return res
}

// SanitizeAttr redige o atributo se sua chave for sensível ou sanitiza seu conteúdo.
func SanitizeAttr(a slog.Attr) slog.Attr {
	if isSensitiveKey(a.Key) {
		return slog.String(a.Key, "[REDACTED]")
	}

	switch a.Value.Kind() {
	case slog.KindString:
		sanitized := SanitizeStringValue(a.Value.String())
		return slog.String(a.Key, sanitized)
	case slog.KindGroup:
		attrs := a.Value.Group()
		sanitizedGroup := make([]slog.Attr, len(attrs))
		for i, sub := range attrs {
			sanitizedGroup[i] = SanitizeAttr(sub)
		}
		return slog.Group(a.Key, anySliceToAny(sanitizedGroup)...)
	default:
		return a
	}
}

func anySliceToAny(attrs []slog.Attr) []any {
	res := make([]any, len(attrs))
	for i, a := range attrs {
		res[i] = a
	}
	return res
}

// SanitizedHandler intercepta registros e atributos do slog para assegurar redação de segredos.
type SanitizedHandler struct {
	inner slog.Handler
}

// NewSanitizedHandler encapsula um handler existente com a política de redação de segredos.
func NewSanitizedHandler(inner slog.Handler) *SanitizedHandler {
	return &SanitizedHandler{inner: inner}
}

func (h *SanitizedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *SanitizedHandler) Handle(ctx context.Context, r slog.Record) error {
	sanitizedRecord := slog.NewRecord(r.Time, r.Level, SanitizeStringValue(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		sanitizedRecord.AddAttrs(SanitizeAttr(a))
		return true
	})
	return h.inner.Handle(ctx, sanitizedRecord)
}

func (h *SanitizedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	sanitizedAttrs := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		sanitizedAttrs[i] = SanitizeAttr(a)
	}
	return &SanitizedHandler{inner: h.inner.WithAttrs(sanitizedAttrs)}
}

func (h *SanitizedHandler) WithGroup(name string) slog.Handler {
	if isSensitiveKey(name) {
		name = "[REDACTED_GROUP]"
	}
	return &SanitizedHandler{inner: h.inner.WithGroup(name)}
}

// ParseLogLevel converte string para slog.Level.
func ParseLogLevel(levelStr string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(levelStr)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// InitLogger configura o logger default da aplicação com base na configuração.
func InitLogger(cfg *config.Config, out io.Writer) *slog.Logger {
	if out == nil {
		out = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level: ParseLogLevel(cfg.LogLevel),
	}

	var inner slog.Handler
	if strings.EqualFold(cfg.LogFormat, "json") {
		inner = slog.NewJSONHandler(out, opts)
	} else {
		inner = slog.NewTextHandler(out, opts)
	}

	sanitized := NewSanitizedHandler(inner)
	logger := slog.New(sanitized)
	slog.SetDefault(logger)
	return logger
}
