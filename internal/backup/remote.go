package backup

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ObjectMetadata representa metadados de um objeto no Object Storage S3.
type ObjectMetadata struct {
	Key          string
	SizeBytes    int64
	ETag         string
	LastModified time.Time
	SHA256Hex    string
	CreatedAt    time.Time
}

// RemoteStorageProvider define o contrato para persistência em Object Storage compatível com S3.
type RemoteStorageProvider interface {
	PutObject(ctx context.Context, key string, data io.Reader, size int64, sha256Hex string, metadata map[string]string) error
	HeadObject(ctx context.Context, key string) (*ObjectMetadata, error)
	ListObjects(ctx context.Context, prefix string) ([]ObjectMetadata, error)
	DeleteObject(ctx context.Context, key string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, *ObjectMetadata, error)
}

// S3Config contém as credenciais e parâmetros de conexão para provedores compatíveis com S3.
type S3Config struct {
	Endpoint   string
	Region     string
	Bucket     string
	AccessKey  string
	SecretKey  string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// S3Provider implementa RemoteStorageProvider em Go puro com assinatura AWS SigV4.
type S3Provider struct {
	config      S3Config
	httpClient  *http.Client
	endpointURL *url.URL
}

// NewS3Provider instancia um novo provedor S3 com validação de endpoint.
func NewS3Provider(cfg S3Config) (*S3Provider, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("backup: bucket S3 não informado")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" {
		return nil, fmt.Errorf("backup: access key S3 não informada")
	}
	if strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, fmt.Errorf("backup: secret key S3 não informada")
	}
	if strings.TrimSpace(cfg.Region) == "" {
		cfg.Region = "us-east-1"
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = "https://s3.us-east-1.amazonaws.com"
	}

	u, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("backup: endpoint S3 inválido %q: %w", cfg.Endpoint, err)
	}
	if u.Scheme != "https" && cfg.HTTPClient == nil {
		return nil, fmt.Errorf("backup: endpoint S3 %q inválido: esquema deve ser https para transporte seguro", cfg.Endpoint)
	}

	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		client = &http.Client{
			Timeout: timeout,
		}
	}

	return &S3Provider{
		config:      cfg,
		httpClient:  client,
		endpointURL: u,
	}, nil
}

// buildURL monta a URL canônica em formato path-style /{bucket}/{key}.
func (p *S3Provider) buildURL(key string) string {
	cleanKey := strings.TrimPrefix(key, "/")
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(p.config.Endpoint, "/"), p.config.Bucket, cleanKey)
}

// PutObject realiza o upload do arquivo para o S3 com assinatura SigV4 e metadados.
func (p *S3Provider) PutObject(ctx context.Context, key string, data io.Reader, size int64, sha256Hex string, metadata map[string]string) error {
	objectURL := p.buildURL(key)

	var bodyReader io.Reader = data
	payloadSHA256 := sha256Hex
	if payloadSHA256 == "" {
		buf, err := io.ReadAll(data)
		if err != nil {
			return fmt.Errorf("backup: falha ao ler corpo para cálculo de SHA256: %w", err)
		}
		hash := sha256.Sum256(buf)
		payloadSHA256 = hex.EncodeToString(hash[:])
		bodyReader = bytes.NewReader(buf)
		size = int64(len(buf))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL, bodyReader)
	if err != nil {
		return fmt.Errorf("backup: falha ao criar requisição PUT: %w", err)
	}

	req.ContentLength = size
	req.Header.Set("Content-Type", "application/x-sqlite3")
	req.Header.Set("x-amz-content-sha256", payloadSHA256)
	req.Header.Set("x-amz-meta-sha256", payloadSHA256)

	for k, v := range metadata {
		headerKey := "x-amz-meta-" + strings.ToLower(strings.TrimSpace(k))
		req.Header.Set(headerKey, strings.TrimSpace(v))
	}

	now := time.Now().UTC()
	if err := p.signRequest(req, payloadSHA256, now); err != nil {
		return fmt.Errorf("backup: falha ao assinar requisição PUT: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("backup: falha no envio HTTP para o S3: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("backup: S3 PutObject retornou status HTTP %d: %s", resp.StatusCode, string(bodySnippet))
	}

	return nil
}

// HeadObject consulta a existência e metadados de um objeto no S3 sem baixar o corpo.
func (p *S3Provider) HeadObject(ctx context.Context, key string) (*ObjectMetadata, error) {
	objectURL := p.buildURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, objectURL, nil)
	if err != nil {
		return nil, fmt.Errorf("backup: falha ao criar requisição HEAD: %w", err)
	}

	emptyHash := emptyPayloadSHA256()
	req.Header.Set("x-amz-content-sha256", emptyHash)

	now := time.Now().UTC()
	if err := p.signRequest(req, emptyHash, now); err != nil {
		return nil, fmt.Errorf("backup: falha ao assinar requisição HEAD: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backup: falha ao executar requisição HEAD no S3: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("backup: objeto %q não encontrado no bucket", key)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backup: S3 HeadObject retornou status HTTP %d", resp.StatusCode)
	}

	size := resp.ContentLength
	etag := strings.Trim(resp.Header.Get("ETag"), "\"")
	sha256Meta := resp.Header.Get("x-amz-meta-sha256")

	var lastMod time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		lastMod, _ = http.ParseTime(lm)
	}

	var createdAt time.Time
	if ca := resp.Header.Get("x-amz-meta-created-at"); ca != "" {
		createdAt, _ = time.Parse(time.RFC3339, ca)
	}

	return &ObjectMetadata{
		Key:          key,
		SizeBytes:    size,
		ETag:         etag,
		LastModified: lastMod.UTC(),
		SHA256Hex:    sha256Meta,
		CreatedAt:    createdAt.UTC(),
	}, nil
}

// GetObject baixa o conteúdo de um objeto do S3.
func (p *S3Provider) GetObject(ctx context.Context, key string) (io.ReadCloser, *ObjectMetadata, error) {
	objectURL := p.buildURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("backup: falha ao criar requisição GET: %w", err)
	}

	emptyHash := emptyPayloadSHA256()
	req.Header.Set("x-amz-content-sha256", emptyHash)

	now := time.Now().UTC()
	if err := p.signRequest(req, emptyHash, now); err != nil {
		return nil, nil, fmt.Errorf("backup: falha ao assinar requisição GET: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("backup: falha ao executar GET no S3: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("backup: objeto %q não encontrado no bucket", key)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, nil, fmt.Errorf("backup: S3 GetObject retornou status HTTP %d: %s", resp.StatusCode, string(bodySnippet))
	}

	meta := &ObjectMetadata{
		Key:       key,
		SizeBytes: resp.ContentLength,
		ETag:      strings.Trim(resp.Header.Get("ETag"), "\""),
		SHA256Hex: resp.Header.Get("x-amz-meta-sha256"),
	}

	return resp.Body, meta, nil
}

// DeleteObject exclui um objeto do S3.
func (p *S3Provider) DeleteObject(ctx context.Context, key string) error {
	objectURL := p.buildURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, objectURL, nil)
	if err != nil {
		return fmt.Errorf("backup: falha ao criar requisição DELETE: %w", err)
	}

	emptyHash := emptyPayloadSHA256()
	req.Header.Set("x-amz-content-sha256", emptyHash)

	now := time.Now().UTC()
	if err := p.signRequest(req, emptyHash, now); err != nil {
		return fmt.Errorf("backup: falha ao assinar requisição DELETE: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("backup: falha ao executar DELETE no S3: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("backup: S3 DeleteObject retornou status HTTP %d: %s", resp.StatusCode, string(bodySnippet))
	}

	return nil
}

// s3ListBucketResult mapeia a estrutura XML de resposta do S3 ListObjectsV2.
type s3ListBucketResult struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Name     string   `xml:"Name"`
	Prefix   string   `xml:"Prefix"`
	Contents []struct {
		Key          string `xml:"Key"`
		LastModified string `xml:"LastModified"`
		ETag         string `xml:"ETag"`
		Size         int64  `xml:"Size"`
	} `xml:"Contents"`
}

// ListObjects lista objetos com prefixo no bucket S3.
func (p *S3Provider) ListObjects(ctx context.Context, prefix string) ([]ObjectMetadata, error) {
	cleanPrefix := strings.Trim(prefix, "/")
	listURL := fmt.Sprintf("%s/%s?list-type=2", strings.TrimRight(p.config.Endpoint, "/"), p.config.Bucket)
	if cleanPrefix != "" {
		listURL += "&prefix=" + url.QueryEscape(cleanPrefix)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("backup: falha ao criar requisição LIST: %w", err)
	}

	emptyHash := emptyPayloadSHA256()
	req.Header.Set("x-amz-content-sha256", emptyHash)

	now := time.Now().UTC()
	if err := p.signRequest(req, emptyHash, now); err != nil {
		return nil, fmt.Errorf("backup: falha ao assinar requisição LIST: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backup: falha ao listar objetos no S3: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("backup: S3 ListObjects retornou status HTTP %d: %s", resp.StatusCode, string(bodySnippet))
	}

	var result s3ListBucketResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("backup: falha ao interpretar XML do S3: %w", err)
	}

	var objects []ObjectMetadata
	for _, item := range result.Contents {
		var lm time.Time
		if item.LastModified != "" {
			lm, _ = time.Parse(time.RFC3339, item.LastModified)
		}
		objects = append(objects, ObjectMetadata{
			Key:          item.Key,
			SizeBytes:    item.Size,
			ETag:         strings.Trim(item.ETag, "\""),
			LastModified: lm.UTC(),
		})
	}

	return objects, nil
}

// signRequest aplica a assinatura AWS Signature Version 4 (SigV4) nos cabeçalhos HTTP.
func (p *S3Provider) signRequest(req *http.Request, payloadSHA256 string, t time.Time) error {
	amzDate := t.Format("20060102T150405Z")
	dateOnly := t.Format("20060102")

	req.Header.Set("x-amz-date", amzDate)
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}

	// 1. Canonical Headers e Signed Headers
	type headerItem struct {
		name  string
		value string
	}
	var headers []headerItem
	for k, vv := range req.Header {
		lowerK := strings.ToLower(strings.TrimSpace(k))
		var cleanValues []string
		for _, v := range vv {
			cleanValues = append(cleanValues, strings.TrimSpace(v))
		}
		headers = append(headers, headerItem{
			name:  lowerK,
			value: strings.Join(cleanValues, ","),
		})
	}

	sort.Slice(headers, func(i, j int) bool {
		return headers[i].name < headers[j].name
	})

	var canonicalHeaders strings.Builder
	var signedHeadersList []string
	for _, h := range headers {
		canonicalHeaders.WriteString(fmt.Sprintf("%s:%s\n", h.name, h.value))
		signedHeadersList = append(signedHeadersList, h.name)
	}
	signedHeaders := strings.Join(signedHeadersList, ";")

	// 2. Canonical URI
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	// 3. Canonical Query String
	canonicalQuery := buildCanonicalQueryString(req.URL.Query())

	// 4. Canonical Request
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders.String(),
		signedHeaders,
		payloadSHA256,
	)

	hashedCanonicalReq := sha256Hex([]byte(canonicalRequest))

	// 5. String to Sign
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateOnly, p.config.Region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credentialScope,
		hashedCanonicalReq,
	)

	// 6. Signature Calculation
	kDate := hmacSHA256([]byte("AWS4"+p.config.SecretKey), []byte(dateOnly))
	kRegion := hmacSHA256(kDate, []byte(p.config.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	// 7. Set Authorization Header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		p.config.AccessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authHeader)

	return nil
}

func buildCanonicalQueryString(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	var keys []string
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		escapedKey := url.QueryEscape(k)
		values := query[k]
		sort.Strings(values)
		if len(values) == 0 {
			parts = append(parts, escapedKey+"=")
		} else {
			for _, v := range values {
				parts = append(parts, escapedKey+"="+url.QueryEscape(v))
			}
		}
	}
	return strings.Join(parts, "&")
}

func emptyPayloadSHA256() string {
	hash := sha256.Sum256([]byte{})
	return hex.EncodeToString(hash[:])
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// FormatContentLength converte int64 para string de cabeçalho.
func FormatContentLength(size int64) string {
	return strconv.FormatInt(size, 10)
}
