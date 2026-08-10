package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/VishalPainjane/objex/internal/auth"
	"github.com/VishalPainjane/objex/internal/cluster"
	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/metrics"
	"github.com/VishalPainjane/objex/internal/object"
	"github.com/VishalPainjane/objex/internal/replication"
	"github.com/VishalPainjane/objex/internal/s3"
)

// Handler serves the S3-compatible HTTP API.
type Handler struct {
	svc                  *object.Service
	logger               *slog.Logger
	readyCheck           ReadyChecker
	publicURL            string
	sigV4Region          string
	presignDefaultExpiry int
	presignMaxExpiry     int
	localNodeID          string
	membership           cluster.Membership
	placement            cluster.Placer
	proxy                *cluster.Proxy
	peerSync             *cluster.PeerSync
	replication          *replication.Coordinator
	internalToken        string
	peerHealth           *cluster.PeerHealthTracker
}

// HandlerConfig configures optional handler settings.
type HandlerConfig struct {
	PublicURL            string
	SigV4Region          string
	PresignDefaultExpiry int
	PresignMaxExpiry     int
	LocalNodeID          string
	Membership           cluster.Membership
	Placement            cluster.Placer
	Proxy                *cluster.Proxy
	PeerSync             *cluster.PeerSync
	Replication          *replication.Coordinator
	InternalToken        string
	PeerHealth           *cluster.PeerHealthTracker
}

// NewHandler creates an API handler.
func NewHandler(svc *object.Service, logger *slog.Logger) *Handler {
	return NewHandlerWithConfig(svc, HandlerConfig{}, logger)
}

// NewHandlerWithPublicURL creates an API handler with a public base URL for multipart complete Location.
func NewHandlerWithPublicURL(svc *object.Service, publicURL string, logger *slog.Logger) *Handler {
	return NewHandlerWithConfig(svc, HandlerConfig{PublicURL: publicURL}, logger)
}

// NewHandlerWithConfig creates an API handler with full configuration.
func NewHandlerWithConfig(svc *object.Service, cfg HandlerConfig, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = "http://localhost:9000"
	}
	if cfg.SigV4Region == "" {
		cfg.SigV4Region = "us-east-1"
	}
	if cfg.PresignDefaultExpiry <= 0 {
		cfg.PresignDefaultExpiry = 3600
	}
	if cfg.PresignMaxExpiry <= 0 {
		cfg.PresignMaxExpiry = 604800
	}
	return &Handler{
		svc:                  svc,
		logger:               logger,
		publicURL:            strings.TrimRight(cfg.PublicURL, "/"),
		sigV4Region:          cfg.SigV4Region,
		presignDefaultExpiry: cfg.PresignDefaultExpiry,
		presignMaxExpiry:     cfg.PresignMaxExpiry,
		localNodeID:          cfg.LocalNodeID,
		membership:           cfg.Membership,
		placement:            cfg.Placement,
		proxy:                cfg.Proxy,
		peerSync:             cfg.PeerSync,
		replication:          cfg.Replication,
		internalToken:        cfg.InternalToken,
		peerHealth:           cfg.PeerHealth,
	}
}

// ServeHTTP dispatches all S3 API routes.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch path {
	case "/health", "/health/live":
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			h.healthLive(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	case "/health/ready":
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			h.healthReady(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	case "/cluster":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.clusterInfo(w, r)
		return
	case "/debug/placement":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.placementDebug(w, r)
		return
	}

	if strings.HasPrefix(path, "/api/presign/") {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.presignObject(w, r)
		return
	}

	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		if r.Method == http.MethodGet {
			h.listBuckets(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		bucket, err := url.PathUnescape(trimmed)
		if err != nil {
			s3.WriteError(w, "InvalidArgument", "Invalid bucket path.", http.StatusBadRequest)
			return
		}
		r.SetPathValue("bucket", bucket)
		h.dispatchBucket(w, r)
		return
	}

	bucket, err := url.PathUnescape(trimmed[:slash])
	if err != nil {
		s3.WriteError(w, "InvalidArgument", "Invalid bucket path.", http.StatusBadRequest)
		return
	}
	key, err := url.PathUnescape(trimmed[slash+1:])
	if err != nil {
		s3.WriteError(w, "InvalidArgument", "Invalid object key path.", http.StatusBadRequest)
		return
	}
	r.SetPathValue("bucket", bucket)
	r.SetPathValue("key", key)
	h.dispatchObject(w, r)
}

// Register mounts the handler on mux (single tree avoids pattern conflicts).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/", h)
}

func (h *Handler) dispatchObject(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		h.putObject(w, r)
	case http.MethodPost:
		h.postObject(w, r)
	case http.MethodGet:
		h.getObject(w, r)
	case http.MethodHead:
		h.headObject(w, r)
	case http.MethodDelete:
		h.deleteObject(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) dispatchBucket(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listObjects(w, r)
	case http.MethodHead:
		h.headBucket(w, r)
	case http.MethodPut:
		h.createBucket(w, r)
	case http.MethodDelete:
		h.deleteBucket(w, r)
	case http.MethodPost:
		h.postBucket(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) listBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := h.svc.ListBuckets(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	s3.WriteListBuckets(w, buckets)
}

func (h *Handler) createBucket(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	if err := h.svc.CreateBucket(r.Context(), bucket); err != nil {
		h.writeError(w, err)
		return
	}
	if h.peerSync != nil {
		if err := h.peerSync.EnsureBucketOnPeers(r.Context(), bucket); err != nil {
			if h.logger != nil {
				h.logger.Warn("peer bucket sync failed", "bucket", bucket, "error", err)
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) headBucket(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	if err := h.svc.HeadBucket(r.Context(), bucket); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteBucket(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	if err := h.svc.DeleteBucket(r.Context(), bucket); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) postBucket(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("delete") {
		h.deleteObjects(w, r)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (h *Handler) deleteObjects(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	if err := h.svc.HeadBucket(r.Context(), bucket); err != nil {
		h.writeError(w, err)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		s3.WriteError(w, "InternalError", "Failed to read request body.", http.StatusInternalServerError)
		return
	}

	keys, err := s3.ParseDeleteObjectKeys(body)
	if err != nil {
		s3.WriteError(w, string(object.CodeMalformedXML), "The XML you provided was not well-formed.", http.StatusBadRequest)
		return
	}
	if len(keys) == 0 {
		s3.WriteError(w, string(object.CodeMalformedXML), "No keys specified.", http.StatusBadRequest)
		return
	}
	if len(keys) > s3.MaxBatchDeleteKeys {
		s3.WriteError(w, string(object.CodeMalformedXML), "The batch delete request may contain a maximum of 1000 keys.", http.StatusBadRequest)
		return
	}

	deleted := make([]string, 0, len(keys))
	var errs []s3.DeleteObjectError
	for _, key := range keys {
		if err := h.deleteObjectKey(r.Context(), r, bucket, key); err != nil {
			if oe, ok := object.AsError(err); ok {
				errs = append(errs, s3.DeleteObjectError{
					Key:     key,
					Code:    string(oe.Code),
					Message: oe.Message,
				})
				continue
			}
			errs = append(errs, s3.DeleteObjectError{
				Key:     key,
				Code:    string(object.CodeInternalError),
				Message: err.Error(),
			})
			continue
		}
		deleted = append(deleted, key)
	}
	s3.WriteDeleteObjectsResult(w, deleted, errs)
}

func (h *Handler) deleteObjectKey(ctx context.Context, r *http.Request, bucket, key string) error {
	if h.proxy != nil && h.placement != nil && h.localNodeID != "" && !auth.IsInternalRequest(r, h.internalToken) {
		result, err := h.placement.Locate(bucket, key)
		if err != nil {
			return err
		}
		if result.Primary.ID != h.localNodeID {
			return h.proxy.ForwardDelete(ctx, result.Primary.Address, bucket, key, h.internalToken)
		}
	}
	if h.replication != nil && h.replication.Enabled() {
		return h.replication.DeleteObject(ctx, bucket, key)
	}
	return h.svc.DeleteObject(ctx, bucket, key)
}

func (h *Handler) listObjects(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	q := r.URL.Query()

	if q.Has("location") {
		s3.WriteBucketLocation(w)
		return
	}
	if q.Has("uploads") {
		uploads, err := h.svc.ListMultipartUploads(r.Context(), bucket)
		if err != nil {
			h.writeError(w, err)
			return
		}
		s3.WriteListMultipartUploads(w, bucket, uploads)
		return
	}
	for _, unsupported := range []string{"versioning", "lifecycle", "policy", "cors", "encryption", "tagging", "acl"} {
		if q.Has(unsupported) {
			s3.WriteError(w, "NotImplemented", "This operation is not yet supported.", http.StatusNotImplemented)
			return
		}
	}

	opts := metadata.ListOptions{
		Prefix:            q.Get("prefix"),
		Delimiter:         q.Get("delimiter"),
		ContinuationToken: q.Get("continuation-token"),
		StartAfter:        q.Get("start-after"),
	}
	if mk := q.Get("max-keys"); mk != "" {
		if n, err := strconv.Atoi(mk); err == nil && n > 0 {
			opts.MaxKeys = n
		}
	}

	result, err := h.svc.ListObjects(r.Context(), bucket, opts)
	if err != nil {
		h.writeError(w, err)
		return
	}

	s3.WriteListObjects(w, s3.ListObjectsParams{
		Bucket:            bucket,
		Prefix:            opts.Prefix,
		Delimiter:         opts.Delimiter,
		ListType2:         q.Get("list-type") == "2",
		ContinuationToken: opts.ContinuationToken,
		StartAfter:        opts.StartAfter,
		Result:            result,
	})
}

func (h *Handler) putObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	if key == "" {
		s3.WriteError(w, "InvalidArgument", "Object key must not be empty", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	if q.Has("partNumber") && q.Has("uploadId") {
		partNumber, err := strconv.Atoi(q.Get("partNumber"))
		if err != nil || partNumber < 1 {
			s3.WriteError(w, "InvalidArgument", "Invalid part number.", http.StatusBadRequest)
			return
		}
		uploadID := q.Get("uploadId")
		if !isUploadID(uploadID) {
			s3.WriteError(w, "NoSuchUpload", "The specified upload does not exist.", http.StatusNotFound)
			return
		}
		etag, err := h.svc.UploadPart(r.Context(), bucket, key, uploadID, partNumber, r.Body)
		if err != nil {
			h.writeError(w, err)
			return
		}
		w.Header().Set("ETag", `"`+etag+`"`)
		w.WriteHeader(http.StatusOK)
		return
	}

	if auth.IsReplicatePut(r, h.internalToken) {
		h.putReplica(w, r, bucket, key)
		return
	}

	if h.maybeForwardObject(w, r, bucket, key, "put_object") {
		return
	}

	copySource := r.Header.Get("x-amz-copy-source")
	if copySource != "" {
		srcBucket, srcKey, ok := parseCopySource(copySource)
		if !ok {
			s3.WriteError(w, "InvalidArgument", "Invalid x-amz-copy-source format.", http.StatusBadRequest)
			return
		}
		etag, err := h.svc.CopyObject(r.Context(), srcBucket, srcKey, bucket, key)
		if err != nil {
			h.writeError(w, err)
			return
		}
		s3.WriteCopyObjectResult(w, etag, time.Now().UTC())
		return
	}

	custom := extractCustomMetadata(r.Header)
	var etag string
	var err error
	if h.replication != nil && h.replication.Enabled() {
		etag, err = h.replication.PutObject(r.Context(), object.PutObjectInput{
			BucketName:     bucket,
			Key:            key,
			ContentType:    r.Header.Get("Content-Type"),
			CustomMetadata: custom,
			Body:           r.Body,
		})
	} else {
		etag, err = h.svc.PutObject(r.Context(), object.PutObjectInput{
			BucketName:     bucket,
			Key:            key,
			ContentType:    r.Header.Get("Content-Type"),
			CustomMetadata: custom,
			Body:           r.Body,
		})
	}
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("ETag", `"`+etag+`"`)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) getObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")

	if uploadID := r.URL.Query().Get("uploadId"); uploadID != "" {
		if !isUploadID(uploadID) {
			s3.WriteError(w, "NoSuchUpload", "The specified upload does not exist.", http.StatusNotFound)
			return
		}
		parts, err := h.svc.ListParts(r.Context(), bucket, key, uploadID)
		if err != nil {
			h.writeError(w, err)
			return
		}
		s3.WriteListParts(w, bucket, key, uploadID, parts)
		return
	}

	if h.maybeForwardObject(w, r, bucket, key, "get_object") {
		return
	}

	if auth.IsReplicateGet(r, h.internalToken) {
		h.getReplica(w, r, bucket, key)
		return
	}

	verify := r.Header.Get("x-objex-verify-integrity") != ""

	var result *object.GetObjectResult
	var err error
	if h.replication != nil && h.replication.Enabled() {
		result, err = h.replication.GetObject(r.Context(), bucket, key, verify)
	} else {
		result, err = h.svc.GetObject(r.Context(), bucket, key, verify)
	}
	if err != nil {
		h.writeError(w, err)
		return
	}
	defer result.Body.Close()

	setCustomMetadata(w.Header(), result.CustomMetadata)
	w.Header().Set("ETag", `"`+result.ETag+`"`)
	w.Header().Set("Last-Modified", result.LastModified.UTC().Format(http.TimeFormat))

	contentType := result.ContentType
	if r.URL.Query().Get("download") == "true" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	if err := serveWithRange(w, r, result.Body, result.Size); err != nil {
		h.logger.Error("get object stream failed", "bucket", bucket, "key", key, "error", err)
	}
}

func (h *Handler) headObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	if h.maybeForwardObject(w, r, bucket, key, "head_object") {
		return
	}

	if auth.IsReplicateHead(r, h.internalToken) {
		h.headReplica(w, r, bucket, key)
		return
	}

	var obj *metadata.Object
	var err error
	if h.replication != nil && h.replication.Enabled() {
		obj, err = h.replication.HeadObject(r.Context(), bucket, key)
	} else {
		obj, err = h.svc.HeadObject(r.Context(), bucket, key)
	}
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("ETag", `"`+obj.ETag+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Last-Modified", obj.UpdatedAt.UTC().Format(http.TimeFormat))
	setCustomMetadata(w.Header(), obj.CustomMetadata)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")

	if uploadID := r.URL.Query().Get("uploadId"); uploadID != "" {
		if !isUploadID(uploadID) {
			s3.WriteError(w, "NoSuchUpload", "The specified upload does not exist.", http.StatusNotFound)
			return
		}
		if err := h.svc.AbortMultipartUpload(r.Context(), bucket, key, uploadID); err != nil {
			h.writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if auth.IsReplicateDelete(r, h.internalToken) {
		h.deleteReplica(w, r, bucket, key)
		return
	}

	if h.maybeForwardObject(w, r, bucket, key, "delete_object") {
		return
	}

	if h.replication != nil && h.replication.Enabled() {
		if err := h.replication.DeleteObject(r.Context(), bucket, key); err != nil {
			h.writeError(w, err)
			return
		}
	} else if err := h.svc.DeleteObject(r.Context(), bucket, key); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) putReplica(w http.ResponseWriter, r *http.Request, bucket, key string) {
	version, err := strconv.ParseInt(r.Header.Get(auth.InternalObjectVersion), 10, 64)
	if err != nil || version < 1 {
		s3.WriteError(w, "InvalidArgument", "Invalid object version.", http.StatusBadRequest)
		return
	}
	expectedETag := r.Header.Get(auth.InternalExpectedETag)
	if expectedETag == "" {
		s3.WriteError(w, "InvalidArgument", "Missing expected ETag.", http.StatusBadRequest)
		return
	}
	err = h.svc.PutReplica(r.Context(), object.PutReplicaInput{
		BucketName:     bucket,
		Key:            key,
		Version:        version,
		ExpectedETag:   expectedETag,
		ContentType:    r.Header.Get("Content-Type"),
		CustomMetadata: extractCustomMetadata(r.Header),
		Body:           r.Body,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteReplica(w http.ResponseWriter, r *http.Request, bucket, key string) {
	version, err := strconv.ParseInt(r.Header.Get(auth.InternalObjectVersion), 10, 64)
	if err != nil || version < 1 {
		s3.WriteError(w, "InvalidArgument", "Invalid object version.", http.StatusBadRequest)
		return
	}
	if err := h.svc.PutTombstoneReplica(r.Context(), bucket, key, version); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) headReplica(w http.ResponseWriter, r *http.Request, bucket, key string) {
	found, obj, err := h.svc.LocalReplicaState(r.Context(), bucket, key)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if !found || obj == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set(auth.InternalObjectVersion, strconv.FormatInt(obj.Version, 10))
	w.Header().Set("ETag", `"`+obj.ETag+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("Content-Type", obj.ContentType)
	if obj.Deleted {
		w.Header().Set(auth.InternalObjectDeleted, "true")
	}
	setCustomMetadata(w.Header(), obj.CustomMetadata)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) getReplica(w http.ResponseWriter, r *http.Request, bucket, key string) {
	found, obj, err := h.svc.LocalReplicaState(r.Context(), bucket, key)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if !found || obj == nil || obj.Deleted {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	rc, size, err := h.svc.OpenStoredObject(r.Context(), bucket, key)
	if err != nil {
		h.writeError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("ETag", `"`+obj.ETag+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Content-Type", obj.ContentType)
	setCustomMetadata(w.Header(), obj.CustomMetadata)
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}

func (h *Handler) postObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	if key == "" {
		s3.WriteError(w, "InvalidArgument", "Object key must not be empty", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	if q.Has("uploads") {
		uploadID, err := h.svc.InitiateMultipartUpload(r.Context(), bucket, key, r.Header.Get("Content-Type"))
		if err != nil {
			h.writeError(w, err)
			return
		}
		s3.WriteInitiateMultipartUpload(w, bucket, key, uploadID)
		return
	}

	uploadID := q.Get("uploadId")
	if uploadID == "" {
		s3.WriteError(w, "InvalidArgument", "Missing uploads or uploadId query parameter.", http.StatusBadRequest)
		return
	}
	if !isUploadID(uploadID) {
		s3.WriteError(w, "NoSuchUpload", "The specified upload does not exist.", http.StatusNotFound)
		return
	}

	parts, err := parseCompleteMultipartBody(r.Body)
	if err != nil {
		s3.WriteError(w, "MalformedXML", "The XML you provided was not well-formed.", http.StatusBadRequest)
		return
	}
	if len(parts) == 0 {
		s3.WriteError(w, "MalformedXML", "You must specify at least one part.", http.StatusBadRequest)
		return
	}

	etag, err := h.svc.CompleteMultipartUpload(r.Context(), bucket, key, uploadID, parts)
	if err != nil {
		h.writeError(w, err)
		return
	}
	location := h.publicURL + "/" + url.PathEscape(bucket) + "/" + escapeKeyPath(key)
	s3.WriteCompleteMultipartUpload(w, bucket, key, location, etag)
}

func (h *Handler) presignObject(w http.ResponseWriter, r *http.Request) {
	cred, ok := auth.CredentialFromContext(r.Context())
	if !ok {
		s3.WriteError(w, "AccessDenied", "No credentials provided.", http.StatusForbidden)
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/api/presign/")
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		s3.WriteError(w, "InvalidArgument", "Object key required.", http.StatusBadRequest)
		return
	}
	bucket, err := url.PathUnescape(trimmed[:slash])
	if err != nil {
		s3.WriteError(w, "InvalidArgument", "Invalid bucket path.", http.StatusBadRequest)
		return
	}
	key, err := url.PathUnescape(trimmed[slash+1:])
	if err != nil {
		s3.WriteError(w, "InvalidArgument", "Invalid object key path.", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	expires := h.presignDefaultExpiry
	if v := q.Get("expires"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			expires = n
		}
	}
	if expires > h.presignMaxExpiry {
		expires = h.presignMaxExpiry
	}
	if expires < 1 {
		expires = 1
	}

	method := "GET"
	if strings.EqualFold(q.Get("method"), "PUT") {
		method = "PUT"
	}

	url, err := auth.GeneratePresignedURL(auth.PresignOptions{
		BaseURL:         h.publicURL,
		Bucket:          bucket,
		Key:             key,
		AccessKeyID:     cred.AccessKeyID,
		SecretAccessKey: cred.SecretAccessKey,
		ExpiresSeconds:  expires,
		Region:          h.sigV4Region,
		Method:          method,
	})
	if err != nil {
		h.logger.Error("presign failed", "error", err)
		s3.WriteError(w, "InternalError", "An internal error occurred.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"url":%q}`, url)
}

type completeMultipartXML struct {
	XMLName xml.Name          `xml:"CompleteMultipartUpload"`
	Parts   []completePartXML `xml:"Part"`
}

type completePartXML struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

func parseCompleteMultipartBody(body io.Reader) ([]metadata.CompletePart, error) {
	var doc completeMultipartXML
	if err := xml.NewDecoder(body).Decode(&doc); err != nil {
		return nil, err
	}
	out := make([]metadata.CompletePart, 0, len(doc.Parts))
	for _, p := range doc.Parts {
		if p.PartNumber < 1 || p.ETag == "" {
			continue
		}
		out = append(out, metadata.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	return out, nil
}

func parseCopySource(raw string) (bucket, key string, ok bool) {
	decoded, err := url.PathUnescape(strings.TrimSpace(raw))
	if err != nil {
		return "", "", false
	}
	decoded = strings.TrimPrefix(decoded, "/")
	slash := strings.Index(decoded, "/")
	if slash < 1 {
		return "", "", false
	}
	return decoded[:slash], decoded[slash+1:], true
}

func isUploadID(id string) bool {
	// UUID: 8-4-4-4-12 hex with dashes
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}

func escapeKeyPath(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	if oe, ok := object.AsError(err); ok {
		s3.WriteError(w, string(oe.Code), oe.Message, oe.StatusCode)
		return
	}
	h.logger.Error("internal error", "error", err)
	s3.WriteError(w, "InternalError", "An internal error occurred.", http.StatusInternalServerError)
}

func extractCustomMetadata(h http.Header) map[string]string {
	out := make(map[string]string)
	for k, vals := range h {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-amz-meta-") && len(vals) > 0 {
			out[lower] = vals[0]
		}
	}
	return out
}

func setCustomMetadata(h http.Header, m map[string]string) {
	for k, v := range m {
		if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") {
			h.Set(k, v)
		}
	}
}

func serveWithRange(w http.ResponseWriter, r *http.Request, body io.ReadCloser, size int64) error {
	rangeHdr := r.Header.Get("Range")
	if rangeHdr == "" {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		_, err := io.Copy(w, body)
		return err
	}

	start, end, ok := parseRange(rangeHdr, size)
	if !ok {
		if size == 0 {
			w.Header().Set("Content-Range", "bytes */0")
		} else {
			w.Header().Set("Content-Range", fmtContentRange(0, size-1, size))
		}
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}

	if seeker, ok := body.(io.Seeker); ok {
		if _, err := seeker.Seek(start, io.SeekStart); err != nil {
			return err
		}
	} else {
		if _, err := io.CopyN(io.Discard, body, start); err != nil {
			return err
		}
	}

	length := end - start + 1
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Content-Range", fmtContentRange(start, end, size))
	w.WriteHeader(http.StatusPartialContent)
	_, err := io.CopyN(w, body, length)
	return err
}

func parseRange(rangeHdr string, size int64) (start, end int64, ok bool) {
	if !strings.HasPrefix(rangeHdr, "bytes=") {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(rangeHdr, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, 0, false // multiple ranges not supported
	}
	if strings.HasPrefix(spec, "-") {
		n, err := strconv.ParseInt(strings.TrimPrefix(spec, "-"), 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true
	}
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	if parts[1] == "" {
		end = size - 1
	} else {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
	}
	if start >= size {
		return 0, 0, false
	}
	if end >= size {
		end = size - 1
	}
	return start, end, true
}

func fmtContentRange(start, end, size int64) string {
	return "bytes " + strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(end, 10) + "/" + strconv.FormatInt(size, 10)
}

// ReadyChecker verifies dependencies for readiness probe.
type ReadyChecker func(ctx context.Context) error

func (h *Handler) healthLive(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) healthReady(w http.ResponseWriter, r *http.Request) {
	if h.readyCheck != nil {
		if err := h.readyCheck(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// SetReadyCheck configures the readiness probe callback.
func (h *Handler) SetReadyCheck(fn ReadyChecker) {
	h.readyCheck = fn
}

// SetPublicURL overrides the public base URL (used in tests).
func (h *Handler) SetPublicURL(u string) {
	if u != "" {
		h.publicURL = strings.TrimRight(u, "/")
	}
}

type clusterNodeJSON struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	Status    string `json:"status"`
	Reachable *bool  `json:"reachable,omitempty"`
}

type clusterInfoJSON struct {
	NodeID string            `json:"node_id"`
	Nodes  []clusterNodeJSON `json:"nodes"`
}

func (h *Handler) clusterInfo(w http.ResponseWriter, _ *http.Request) {
	if h.membership == nil {
		http.Error(w, "cluster not configured", http.StatusNotFound)
		return
	}
	nodes := h.membership.ListNodes()
	out := clusterInfoJSON{
		NodeID: h.localNodeID,
		Nodes:  make([]clusterNodeJSON, len(nodes)),
	}
	for i, n := range nodes {
		node := clusterNodeJSON{
			ID:      n.ID,
			Address: n.Address,
			Status:  string(n.Status),
		}
		if h.peerHealth != nil && n.ID != h.localNodeID {
			ok := h.peerHealth.IsReachable(n.ID)
			node.Reachable = &ok
		}
		out.Nodes[i] = node
	}
	w.Header().Set("Content-Type", "application/json")
	if err := jsonMarshal(w, out); err != nil {
		h.logger.Error("cluster json encode failed", "error", err)
	}
}

type placementDebugJSON struct {
	Bucket   string   `json:"bucket"`
	Key      string   `json:"key"`
	Primary  string   `json:"primary"`
	Replicas []string `json:"replicas,omitempty"`
}

func (h *Handler) placementDebug(w http.ResponseWriter, r *http.Request) {
	if h.placement == nil {
		http.Error(w, "placement not configured", http.StatusNotFound)
		return
	}
	bucket := r.URL.Query().Get("bucket")
	key := r.URL.Query().Get("key")
	if bucket == "" || key == "" {
		http.Error(w, "bucket and key query parameters required", http.StatusBadRequest)
		return
	}
	metrics.RecordPlacement("debug")
	result, err := h.placement.Locate(bucket, key)
	if err != nil {
		if errors.Is(err, cluster.ErrEmptyCluster) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	replicaIDs := make([]string, len(result.Replicas))
	for i, n := range result.Replicas {
		replicaIDs[i] = n.ID
	}
	out := placementDebugJSON{
		Bucket:   bucket,
		Key:      key,
		Primary:  result.Primary.ID,
		Replicas: replicaIDs,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := jsonMarshal(w, out); err != nil {
		h.logger.Error("placement debug json encode failed", "error", err)
	}
}

func jsonMarshal(w http.ResponseWriter, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	return enc.Encode(v)
}
