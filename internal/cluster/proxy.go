package cluster

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Proxy forwards HTTP requests to a peer node when the local node is not primary.
type Proxy struct {
	internalToken string
	client        *http.Client
}

// NewProxy creates an HTTP forwarder for inter-node requests.
func NewProxy(internalToken string, client *http.Client) *Proxy {
	if client == nil {
		client = http.DefaultClient
	}
	return &Proxy{
		internalToken: internalToken,
		client:        client,
	}
}

// ForwardDelete issues an internal DELETE for one object on a peer node.
func (p *Proxy) ForwardDelete(ctx context.Context, peerAddress, bucket, key, internalToken string) error {
	if internalToken == "" {
		return fmt.Errorf("cluster internal token not configured")
	}
	path := "/" + url.PathEscape(bucket) + "/" + objectURLPath(key)
	target := nodeBaseURL(peerAddress) + path
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-ObjeX-Internal-Token", internalToken)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("forward delete to %s: %w", peerAddress, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("forward delete to %s: status %d", peerAddress, resp.StatusCode)
	}
	return nil
}

// Forward sends the request to peerAddress and copies the response to w.
func (p *Proxy) Forward(w http.ResponseWriter, r *http.Request, peerAddress string) error {
	if p.internalToken == "" {
		return fmt.Errorf("cluster internal token not configured")
	}

	targetURL := nodeBaseURL(peerAddress) + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		return err
	}

	copyHeader(outReq.Header, r.Header)
	outReq.Header.Set("X-ObjeX-Internal-Token", p.internalToken)
	outReq.ContentLength = r.ContentLength

	resp, err := p.client.Do(outReq)
	if err != nil {
		return fmt.Errorf("forward to %s: %w", peerAddress, err)
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

// PeerSync replicates lightweight cluster metadata (e.g. buckets) to peers.
type PeerSync struct {
	membership    Membership
	localID       string
	internalToken string
	client        *http.Client
}

// NewPeerSync creates a peer synchronizer for cluster-wide bucket metadata.
func NewPeerSync(membership Membership, localID, internalToken string, client *http.Client) *PeerSync {
	if client == nil {
		client = http.DefaultClient
	}
	return &PeerSync{
		membership:    membership,
		localID:       localID,
		internalToken: internalToken,
		client:        client,
	}
}

// EnsureBucketOnPeers creates the bucket on all other cluster nodes.
func (ps *PeerSync) EnsureBucketOnPeers(ctx context.Context, bucket string) error {
	if ps.internalToken == "" {
		return fmt.Errorf("cluster internal token not configured")
	}
	path := "/" + url.PathEscape(bucket)
	for _, node := range ps.membership.ListNodes() {
		if node.ID == ps.localID {
			continue
		}
		target := nodeBaseURL(node.Address) + path
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-ObjeX-Internal-Token", ps.internalToken)
		resp, err := ps.client.Do(req)
		if err != nil {
			return fmt.Errorf("peer %s bucket sync: %w", node.ID, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("peer %s bucket sync: status %d", node.ID, resp.StatusCode)
		}
	}
	return nil
}

func nodeBaseURL(address string) string {
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return strings.TrimRight(address, "/")
	}
	return "http://" + address
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		if isHopByHopHeader(k) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func isHopByHopHeader(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailers", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

func objectURLPath(key string) string {
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
