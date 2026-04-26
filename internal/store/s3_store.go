package store

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
	"path"
	"sort"
	"strings"
	"time"
)

type S3Options struct {
	Endpoint        string
	Region          string
	Bucket          string
	Prefix          string
	ForcePathStyle  bool
	AccessKeyID     string
	AccessKeySecret string
}

type S3BlobStore struct {
	endpoint        *url.URL
	region          string
	bucket          string
	prefix          string
	forcePathStyle  bool
	accessKeyID     string
	accessKeySecret string
	client          *http.Client
}

func NewS3BlobStore(opts S3Options) (*S3BlobStore, error) {
	if opts.Endpoint == "" {
		return nil, fmt.Errorf("s3 endpoint is required")
	}
	if !strings.Contains(opts.Endpoint, "://") {
		opts.Endpoint = "https://" + opts.Endpoint
	}
	u, err := url.Parse(opts.Endpoint)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid s3 endpoint %q", opts.Endpoint)
	}
	if opts.Region == "" {
		opts.Region = "us-east-1"
	}
	if opts.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}
	if opts.AccessKeyID == "" || opts.AccessKeySecret == "" {
		return nil, fmt.Errorf("s3 credentials are required")
	}

	return &S3BlobStore{
		endpoint:        u,
		region:          opts.Region,
		bucket:          opts.Bucket,
		prefix:          cleanS3Prefix(opts.Prefix),
		forcePathStyle:  opts.ForcePathStyle,
		accessKeyID:     opts.AccessKeyID,
		accessKeySecret: opts.AccessKeySecret,
		client:          http.DefaultClient,
	}, nil
}

func (s *S3BlobStore) Has(ctx context.Context, sha256 string) (bool, error) {
	return s.HasObject(ctx, s.blobKey(sha256))
}

func (s *S3BlobStore) Put(ctx context.Context, sha256 string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	actual := hexSHA256(data)
	if actual != sha256 {
		return fmt.Errorf("put payload sha256 mismatch: want %s got %s", sha256, actual)
	}
	return s.PutObject(ctx, s.blobKey(sha256), bytes.NewReader(data))
}

func (s *S3BlobStore) Get(ctx context.Context, sha256 string) (io.ReadCloser, error) {
	return s.GetObject(ctx, s.blobKey(sha256))
}

func (s *S3BlobStore) Delete(ctx context.Context, sha256 string) error {
	return s.DeleteObject(ctx, s.blobKey(sha256))
}

func (s *S3BlobStore) HasObject(ctx context.Context, key string) (bool, error) {
	resp, err := s.do(ctx, http.MethodHead, s.objectKey(key), emptySHA256, nil, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return false, fmt.Errorf("s3 HEAD failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func (s *S3BlobStore) PutObject(ctx context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	payloadHash := hexSHA256(data)

	resp, err := s.do(ctx, http.MethodPut, s.objectKey(key), payloadHash, bytes.NewReader(data), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("s3 PUT failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (s *S3BlobStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.do(ctx, http.MethodGet, s.objectKey(key), emptySHA256, nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.Body, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return nil, fmt.Errorf("s3 GET failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (s *S3BlobStore) DeleteObject(ctx context.Context, key string) error {
	resp, err := s.do(ctx, http.MethodDelete, s.objectKey(key), emptySHA256, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("s3 DELETE failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (s *S3BlobStore) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	var out []string
	logicalPrefix := cleanS3Prefix(prefix)
	remotePrefix := s.objectKey(logicalPrefix)
	if logicalPrefix == "" {
		remotePrefix = s.prefix
	}

	continuation := ""
	for {
		q := url.Values{}
		q.Set("list-type", "2")
		if remotePrefix != "" {
			q.Set("prefix", remotePrefix)
		}
		if continuation != "" {
			q.Set("continuation-token", continuation)
		}

		resp, err := s.do(ctx, http.MethodGet, "", emptySHA256, nil, q)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("s3 LIST failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var parsed listBucketResult
		if err := xml.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("parse s3 LIST response: %w", err)
		}
		for _, c := range parsed.Contents {
			logical := s.trimStorePrefix(c.Key)
			if logical == "" {
				continue
			}
			out = append(out, logical)
		}
		if !parsed.IsTruncated || parsed.NextContinuationToken == "" {
			break
		}
		continuation = parsed.NextContinuationToken
	}

	return out, nil
}

type listBucketResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	IsTruncated           bool     `xml:"IsTruncated"`
	NextContinuationToken string   `xml:"NextContinuationToken"`
	Contents              []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

func (s *S3BlobStore) blobKey(sha string) string {
	if len(sha) < 4 {
		return joinS3("sha256", sha+".blob")
	}
	return joinS3("sha256", sha[0:2], sha[2:4], sha+".blob")
}

func (s *S3BlobStore) objectKey(key string) string {
	key = cleanS3Prefix(key)
	return joinS3(s.prefix, key)
}

func (s *S3BlobStore) trimStorePrefix(remoteKey string) string {
	remoteKey = cleanS3Prefix(remoteKey)
	if s.prefix == "" {
		return remoteKey
	}
	prefix := strings.TrimSuffix(s.prefix, "/") + "/"
	if !strings.HasPrefix(remoteKey, prefix) {
		return ""
	}
	return strings.TrimPrefix(remoteKey, prefix)
}

func (s *S3BlobStore) do(ctx context.Context, method string, key string, payloadHash string, body io.Reader, query url.Values) (*http.Response, error) {
	u := *s.endpoint

	if s.forcePathStyle {
		u.Path = joinURLPath(u.Path, s.bucket, key)
	} else {
		u.Host = s.bucket + "." + u.Host
		u.Path = joinURLPath(u.Path, key)
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = body
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	shortDate := now.Format("20060102")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("Host", req.URL.Host)

	auth := s.authorization(method, req.URL, req.URL.Host, payloadHash, amzDate, shortDate)
	req.Header.Set("Authorization", auth)

	return s.client.Do(req)
}

func (s *S3BlobStore) authorization(method string, u *url.URL, host string, payloadHash string, amzDate string, shortDate string) string {
	canonicalHeaders := "host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI(u),
		canonicalQuery(u),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := shortDate + "/" + s.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	signingKey := sigV4SigningKey(s.accessKeySecret, shortDate, s.region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	return "AWS4-HMAC-SHA256 Credential=" + s.accessKeyID + "/" + scope +
		", SignedHeaders=" + signedHeaders +
		", Signature=" + signature
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func canonicalURI(u *url.URL) string {
	if u.EscapedPath() == "" {
		return "/"
	}
	return u.EscapedPath()
}

func canonicalQuery(u *url.URL) string {
	q := u.Query()
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		values := q[k]
		sort.Strings(values)
		for _, v := range values {
			parts = append(parts, uriEncode(k)+"="+uriEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

func cleanS3Prefix(prefix string) string {
	prefix = strings.ReplaceAll(prefix, "\\", "/")
	prefix = strings.Trim(prefix, "/")
	if prefix == "." {
		return ""
	}
	return prefix
}

func joinS3(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return path.Join(cleaned...)
}

func uriEncode(v string) string {
	return strings.ReplaceAll(url.QueryEscape(v), "+", "%20")
}

func joinURLPath(base string, parts ...string) string {
	all := []string{}
	base = strings.Trim(base, "/")
	if base != "" {
		all = append(all, base)
	}
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			all = append(all, escapeS3Path(p))
		}
	}
	if len(all) == 0 {
		return "/"
	}
	return "/" + strings.Join(all, "/")
}

func escapeS3Path(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sigV4SigningKey(secret string, date string, region string, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}
