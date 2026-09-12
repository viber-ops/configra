package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

const (
	maxResponseBytes = 64 << 10
	maxFeishuBytes   = 20 << 10
	maxRetryAfter    = 24 * time.Hour
)

var errDestinationBlocked = errors.New("Notification Destination address is blocked")

type Config struct {
	AllowedInternalHosts []string
	AllowedCIDRs         []string
	RootCAs              *x509.CertPool
}

type AttemptResult struct {
	Status        mysqlstore.NotificationDeliveryStatus
	HTTPStatus    int
	LatencyMS     uint32
	ErrorCode     string
	RetryAfter    time.Duration
	HasRetryAfter bool
}

type Sender struct {
	client     *http.Client
	now        func() time.Time
	feishuHost string
	feishuPort string
}

func NewSender(config Config) (*Sender, error) {
	policy, err := newAddressPolicy(config.AllowedInternalHosts, config.AllowedCIDRs)
	if err != nil {
		return nil, err
	}
	var roots *x509.CertPool
	if config.RootCAs != nil {
		roots = config.RootCAs.Clone()
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            policy.dialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           20,
		MaxIdleConnsPerHost:    4,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		DisableCompression:     true,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
		},
	}
	return &Sender{
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		now:        time.Now,
		feishuHost: "open.feishu.cn",
		feishuPort: "443",
	}, nil
}

func ValidateNetworkPolicy(hosts, cidrs []string) error {
	_, err := newAddressPolicy(hosts, cidrs)
	return err
}

func (sender *Sender) CloseIdleConnections() {
	if sender != nil && sender.client != nil {
		sender.client.CloseIdleConnections()
	}
}

func (sender *Sender) Send(ctx context.Context, target mysqlstore.NotificationTarget, event Event) AttemptResult {
	if !target.Active {
		return AttemptResult{Status: mysqlstore.NotificationDeliveryDead, ErrorCode: "destination_inactive"}
	}
	if target.ErrorCode != "" {
		return AttemptResult{Status: mysqlstore.NotificationDeliveryDead, ErrorCode: target.ErrorCode}
	}
	if sender == nil || sender.client == nil || target.Attempt < 1 || target.Attempt > 8 || !validEvent(event) {
		return AttemptResult{Status: mysqlstore.NotificationDeliveryDead, ErrorCode: "invalid_delivery"}
	}
	parsed, err := parseDestinationURL(target.Provider, target.URL, sender.feishuHost, sender.feishuPort)
	if err != nil {
		return AttemptResult{Status: mysqlstore.NotificationDeliveryDead, ErrorCode: "invalid_destination"}
	}
	timestamp := strconv.FormatInt(sender.now().UTC().Unix(), 10)
	body, err := renderBody(target.Provider, target.Secret, timestamp, event)
	if err != nil {
		return AttemptResult{Status: mysqlstore.NotificationDeliveryDead, ErrorCode: "invalid_payload"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return AttemptResult{Status: mysqlstore.NotificationDeliveryDead, ErrorCode: "invalid_destination"}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Configra-Notification/1")
	if target.Provider == mysqlstore.NotificationGenericWebhook {
		request.Header.Set("X-Configra-Timestamp", timestamp)
		request.Header.Set("X-Configra-Delivery", event.DeliveryID)
		request.Header.Set("X-Configra-Event", event.Type)
		if target.Secret != "" {
			request.Header.Set("X-Configra-Signature", genericSignature(target.Secret, timestamp, event.DeliveryID, event.Type, body))
		}
	}
	started := time.Now()
	response, err := sender.client.Do(request)
	latency := elapsedMilliseconds(started)
	if err != nil {
		if errors.Is(err, errDestinationBlocked) {
			return AttemptResult{Status: mysqlstore.NotificationDeliveryDead, LatencyMS: latency, ErrorCode: "destination_blocked"}
		}
		return AttemptResult{Status: mysqlstore.NotificationDeliveryRetrying, LatencyMS: latency, ErrorCode: "network_error"}
	}
	defer response.Body.Close()
	result := classifyHTTP(response, sender.now().UTC())
	result.LatencyMS = latency
	if result.Status != mysqlstore.NotificationDeliverySucceeded || target.Provider == mysqlstore.NotificationGenericWebhook {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes+1))
		return result
	}
	return classifyFeishuResponse(response.Body, result)
}

func renderBody(provider mysqlstore.NotificationProvider, secret, timestamp string, event Event) ([]byte, error) {
	switch provider {
	case mysqlstore.NotificationGenericWebhook:
		return json.Marshal(event)
	case mysqlstore.NotificationFeishuBot:
		body := struct {
			Timestamp string `json:"timestamp,omitempty"`
			Sign      string `json:"sign,omitempty"`
			Message   string `json:"msg_type"`
			Content   struct {
				Text string `json:"text"`
			} `json:"content"`
		}{Message: "text"}
		body.Content.Text = feishuText(event)
		if secret != "" {
			body.Timestamp = timestamp
			body.Sign = feishuSignature(secret, timestamp)
		}
		encoded, err := json.Marshal(body)
		if err != nil || len(encoded) > maxFeishuBytes {
			return nil, errors.New("invalid Feishu body")
		}
		return encoded, nil
	default:
		return nil, errors.New("unknown Notification provider")
	}
}

func genericSignature(secret, timestamp, delivery, eventType string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "v1\n%s\n%s\n%s\n", timestamp, delivery, eventType)
	_, _ = mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func feishuSignature(secret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func feishuText(event Event) string {
	parts := []string{
		"Configra " + event.Type,
		"Operation: " + event.OperationID,
		"Resource: " + event.ResourceType + "/" + event.Resource,
		"Actor: " + event.Actor.Type + "/" + event.Actor.ID,
	}
	if event.Environment != "" {
		parts = append(parts, "Environment: "+event.Environment)
	}
	if event.Namespace != "" {
		parts = append(parts, "Namespace: "+event.Namespace)
	}
	if event.Revision != 0 {
		parts = append(parts, "Revision: "+strconv.FormatUint(event.Revision, 10))
	}
	return strings.Join(parts, "\n")
}

func classifyHTTP(response *http.Response, now time.Time) AttemptResult {
	result := AttemptResult{HTTPStatus: response.StatusCode}
	switch {
	case response.StatusCode >= 200 && response.StatusCode <= 299:
		result.Status = mysqlstore.NotificationDeliverySucceeded
	case response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500:
		result.Status = mysqlstore.NotificationDeliveryRetrying
		result.ErrorCode = "http_" + strconv.Itoa(response.StatusCode)
		result.RetryAfter, result.HasRetryAfter = parseRetryAfter(response.Header, now)
	default:
		result.Status = mysqlstore.NotificationDeliveryDead
		result.ErrorCode = "http_" + strconv.Itoa(response.StatusCode)
	}
	return result
}

func classifyFeishuResponse(body io.Reader, result AttemptResult) AttemptResult {
	encoded, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil || len(encoded) > maxResponseBytes {
		result.Status = mysqlstore.NotificationDeliveryDead
		result.ErrorCode = "invalid_provider_response"
		return result
	}
	var response struct {
		Code       *int `json:"code"`
		StatusCode *int `json:"StatusCode"`
	}
	if json.Unmarshal(encoded, &response) != nil {
		result.Status = mysqlstore.NotificationDeliveryDead
		result.ErrorCode = "invalid_provider_response"
		return result
	}
	code := response.Code
	if code == nil {
		code = response.StatusCode
	}
	if code == nil {
		result.Status = mysqlstore.NotificationDeliveryDead
		result.ErrorCode = "invalid_provider_response"
		return result
	}
	if *code == 0 {
		return result
	}
	result.ErrorCode = "feishu_" + strconv.Itoa(*code)
	if *code == 11232 {
		result.Status = mysqlstore.NotificationDeliveryRetrying
	} else {
		result.Status = mysqlstore.NotificationDeliveryDead
	}
	return result
}

func parseRetryAfter(header http.Header, now time.Time) (time.Duration, bool) {
	values := header.Values("Retry-After")
	if len(values) != 1 {
		return 0, false
	}
	value := strings.TrimSpace(values[0])
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds >= uint64(maxRetryAfter/time.Second) {
			return maxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	date, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := date.Sub(now)
	if delay < 0 {
		delay = 0
	}
	if delay > maxRetryAfter {
		delay = maxRetryAfter
	}
	return delay, true
}

func elapsedMilliseconds(started time.Time) uint32 {
	value := time.Since(started).Milliseconds()
	if value <= 0 {
		return 0
	}
	if value > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(value)
}

func validEvent(event Event) bool {
	_, deliveryErr := hex.DecodeString(event.DeliveryID)
	return event.SchemaVersion == 1 && len(event.DeliveryID) == 32 && deliveryErr == nil && validEventType(event.Type) &&
		!event.Time.IsZero() && event.OperationID != "" && event.Outcome == mysqlstore.OutcomeSuccess &&
		(event.Actor.Type == "user" || event.Actor.Type == "system") && event.Actor.ID != "" &&
		event.ResourceType != "" && event.Resource != ""
}

func ValidateDestinationURL(provider mysqlstore.NotificationProvider, value string) error {
	_, err := parseDestinationURL(provider, value, "open.feishu.cn", "443")
	return err
}

func parseDestinationURL(provider mysqlstore.NotificationProvider, value, feishuHost, feishuPort string) (*url.URL, error) {
	if value == "" || len(value) > 8<<10 {
		return nil, errors.New("invalid Notification URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" || strings.Contains(parsed.Hostname(), "%") ||
		!validASCIIHost(parsed.Hostname()) || strings.HasSuffix(parsed.Host, ":") {
		return nil, errors.New("invalid Notification URL")
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return nil, errors.New("invalid Notification URL port")
		}
	}
	switch provider {
	case mysqlstore.NotificationGenericWebhook:
	case mysqlstore.NotificationFeishuBot:
		port := parsed.Port()
		if port == "" {
			port = "443"
		}
		token := strings.TrimPrefix(parsed.EscapedPath(), "/open-apis/bot/v2/hook/")
		if normalizeHost(parsed.Hostname()) != normalizeHost(feishuHost) || port != feishuPort ||
			token == "" || token == parsed.EscapedPath() || strings.Contains(token, "/") || parsed.RawQuery != "" {
			return nil, errors.New("invalid Feishu Bot URL")
		}
	default:
		return nil, errors.New("invalid Notification provider")
	}
	return parsed, nil
}

type addressPolicy struct {
	allowedHosts map[string]struct{}
	allowedCIDRs []netip.Prefix
}

func newAddressPolicy(hosts, cidrs []string) (addressPolicy, error) {
	policy := addressPolicy{allowedHosts: make(map[string]struct{}, len(hosts))}
	for _, host := range hosts {
		host = normalizeHost(host)
		if host == "" || !validASCIIHost(host) {
			return addressPolicy{}, errors.New("invalid allowed Notification Host")
		}
		if address, err := netip.ParseAddr(host); err == nil && isHardDenied(address.Unmap()) {
			return addressPolicy{}, errors.New("allowed Notification Host is always blocked")
		}
		policy.allowedHosts[host] = struct{}{}
	}
	for _, value := range cidrs {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Addr().Is4In6() || prefix.Addr().Zone() != "" {
			return addressPolicy{}, errors.New("invalid allowed Notification CIDR")
		}
		prefix = prefix.Masked()
		for _, blocked := range hardDeniedPrefixes {
			if prefixesOverlap(prefix, blocked) {
				return addressPolicy{}, errors.New("allowed Notification CIDR overlaps an always-blocked range")
			}
		}
		policy.allowedCIDRs = append(policy.allowedCIDRs, prefix)
	}
	return policy, nil
}

func (policy addressPolicy) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || strings.Contains(host, "%") {
		return nil, errDestinationBlocked
	}
	originalHost := normalizeHost(host)
	dialer := net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	dialer.ControlContext = func(_ context.Context, _, actualAddress string, _ syscall.RawConn) error {
		actualHost, actualPort, err := net.SplitHostPort(actualAddress)
		if err != nil || strings.Contains(actualHost, "%") {
			return errDestinationBlocked
		}
		resolved, err := netip.ParseAddr(actualHost)
		portNumber, portErr := strconv.ParseUint(actualPort, 10, 16)
		if err != nil || portErr != nil || portNumber == 0 || !policy.allows(originalHost, resolved.Unmap(), uint16(portNumber)) {
			return errDestinationBlocked
		}
		return nil
	}
	return dialer.DialContext(ctx, network, address)
}

func (policy addressPolicy) allows(host string, address netip.Addr, port uint16) bool {
	if isHardDenied(address) {
		return false
	}
	internalAllowed := false
	if _, ok := policy.allowedHosts[normalizeHost(host)]; ok {
		internalAllowed = true
	}
	for _, prefix := range policy.allowedCIDRs {
		if prefix.Contains(address) {
			internalAllowed = true
			break
		}
	}
	if internalAllowed {
		return true
	}
	if port != 443 {
		return false
	}
	if !address.IsValid() || !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range specialUsePrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func validASCIIHost(host string) bool {
	host = normalizeHost(host)
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Zone() == ""
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func validEventType(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') &&
			character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func isHardDenied(address netip.Addr) bool {
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() {
		return true
	}
	for _, prefix := range hardDeniedPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func prefixesOverlap(left, right netip.Prefix) bool {
	return left.Addr().BitLen() == right.Addr().BitLen() &&
		(left.Contains(right.Addr()) || right.Contains(left.Addr()))
}

var hardDeniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

var specialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}
