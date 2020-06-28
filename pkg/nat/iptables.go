package nat

import (
	"fmt"
	"github.com/rs/zerolog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const (
	SO_ORIGINAL_DST = 80
)

type Iptables struct {
	logger zerolog.Logger
}

func NewIptables(logger zerolog.Logger) *Iptables {
	return &Iptables{
		logger: logger,
	}
}

func (p *Iptables) Setup(proxyPort int, subnets []string) error {
	table := "nat"
	chain := fmt.Sprintf("tagane-pid%d", os.Getpid())

	if err := p.iptables([]string{"-t", table, "-N", chain}); err != nil {
		return fmt.Errorf("failed to create a chain: %w", err)
	}

	if err := p.iptables([]string{"-t", table, "-F", chain}); err != nil {
		return fmt.Errorf("failed to flush a chain: %w", err)
	}

	if err := p.iptables([]string{"-t", table, "-I", "OUTPUT", "1", "-j", chain}); err != nil {
		return fmt.Errorf("failed to insert a jump rule to OUTPUT chain: %w", err)
	}

	if err := p.iptables([]string{"-t", table, "-I", "PREROUTING", "1", "-j", chain}); err != nil {
		return fmt.Errorf("failed to insert a jump rule to OUTPUT chain: %w", err)
	}

	if err := p.iptables([]string{"-t", table, "-A", chain, "-j", "RETURN", "-m", "addrtype", "--dst-type", "LOCAL"}); err != nil {
		return fmt.Errorf("failed to add a rule to return dst==local: %w", err)
	}

	for _, subnet := range subnets {
		if err := p.iptables([]string{"-t", table, "-A", chain, "-j", "REDIRECT", "--dest", subnet, "-p", "tcp", "--to-ports", strconv.Itoa(proxyPort)}); err != nil {
			return fmt.Errorf("failed to add a rule to return dst==local: %w", err)
		}
	}

	return nil
}

func (p *Iptables) Shutdown() error {
	table := "nat"
	chain := fmt.Sprintf("tagane-pid%d", os.Getpid())

	if err := p.iptables([]string{"-t", table, "-D", "OUTPUT", "-j", chain}); err != nil {
		return fmt.Errorf("failed to delete a jump rule to OUTPUT chain: %w", err)
	}

	if err := p.iptables([]string{"-t", table, "-D", "PREROUTING", "-j", chain}); err != nil {
		return fmt.Errorf("failed to delete a jump rule to OUTPUT chain: %w", err)
	}

	if err := p.iptables([]string{"-t", table, "-F", chain}); err != nil {
		return fmt.Errorf("failed to flush a chain: %w", err)
	}

	if err := p.iptables([]string{"-t", table, "-X", chain}); err != nil {
		return fmt.Errorf("failed to delete a chain: %w", err)
	}

	return nil
}

func (p *Iptables) GetNATDestination(conn *net.TCPConn) (string, *net.TCPConn, error) {
	// https://gist.github.com/cannium/55ec625516a24da8f547aa2d93f49ecf
	f, err := conn.File()
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	addr, err := syscall.GetsockoptIPv6Mreq(int(f.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err != nil {
		return "", nil, err
	}

	newConn, err := net.FileConn(f)
	if err != nil {
		return "", nil, err
	}

	newTCPConn, ok := newConn.(*net.TCPConn)
	if !ok {
		panic("BUG: not TCPConn")
	}

	dest := fmt.Sprintf("%d.%d.%d.%d:%d",
		addr.Multiaddr[4],
		addr.Multiaddr[5],
		addr.Multiaddr[6],
		addr.Multiaddr[7],
		uint16(addr.Multiaddr[2])<<8+uint16(addr.Multiaddr[3]))

	return dest, newTCPConn, nil
}

func (p *Iptables) iptables(args []string) error {
	c := exec.Command("iptables", args...)
	p.logger.Debug().Strs("args", c.Args).Msg("Running iptables command")
	return c.Run()
}
