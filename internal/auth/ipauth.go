package auth

import (
	"fmt"
	"net"
	"strings"

	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
)

// IPAuthenticator handles IP-based authentication (allowlist)
type IPAuthenticator struct {
	allowedIPs   []string
	allowedCIDRs []*net.IPNet
	logger       *logger.Logger
}

// NewIPAuthenticator creates a new IP authenticator
func NewIPAuthenticator(allowedIPs []string, logger *logger.Logger) (*IPAuthenticator, error) {
	auth := &IPAuthenticator{
		allowedIPs: []string{},
		logger:     logger.WithComponent("auth-ip"),
	}

	// Parse allowed IPs and CIDR ranges
	for _, ipStr := range allowedIPs {
		ipStr = strings.TrimSpace(ipStr)
		if ipStr == "" {
			continue
		}

		// Check if it's a CIDR range
		if strings.Contains(ipStr, "/") {
			_, cidr, err := net.ParseCIDR(ipStr)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %s: %w", ipStr, err)
			}
			auth.allowedCIDRs = append(auth.allowedCIDRs, cidr)
		} else {
			// Individual IP
			ip := net.ParseIP(ipStr)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP address: %s", ipStr)
			}
			auth.allowedIPs = append(auth.allowedIPs, ipStr)
		}
	}

	auth.logger.Infof("Initialized IP authenticator with %d IPs and %d CIDR ranges",
		len(auth.allowedIPs), len(auth.allowedCIDRs))

	return auth, nil
}

// IsAllowed checks if an IP is in the allowlist
func (a *IPAuthenticator) IsAllowed(ipStr string) bool {
	// Parse the IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		a.logger.Warnf("Invalid IP address: %s", ipStr)
		return false
	}

	// Check individual IPs
	for _, allowedIP := range a.allowedIPs {
		if allowedIP == ipStr {
			a.logger.WithClientIP(ipStr).Debug("IP authenticated via exact match")
			return true
		}
	}

	// Check CIDR ranges
	for _, cidr := range a.allowedCIDRs {
		if cidr.Contains(ip) {
			a.logger.WithClientIP(ipStr).Debugf("IP authenticated via CIDR: %s", cidr.String())
			return true
		}
	}

	a.logger.WithClientIP(ipStr).Debug("IP not in allowlist")
	return false
}

// GetAllowedCount returns the number of allowed IPs and CIDR ranges
func (a *IPAuthenticator) GetAllowedCount() (ips int, cidrs int) {
	return len(a.allowedIPs), len(a.allowedCIDRs)
}
