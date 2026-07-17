package brute

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func checkSSH(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func checkFTP(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	reader := bufio.NewReader(conn)
	line, _ := reader.ReadString('\n')
	if !strings.HasPrefix(line, "220") {
		return false
	}

	fmt.Fprintf(conn, "USER %s\r\n", user)
	line, _ = reader.ReadString('\n')
	if !strings.HasPrefix(line, "331") && !strings.HasPrefix(line, "230") {
		return false
	}
	if strings.HasPrefix(line, "230") {
		return true
	}

	fmt.Fprintf(conn, "PASS %s\r\n", pass)
	line, _ = reader.ReadString('\n')
	return strings.HasPrefix(line, "230")
}

func checkMySQL(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n < 5 {
		return false
	}

	// MySQL handshake: read greeting, extract auth data, send auth response
	// Simplified: just check if we can connect. For real auth, use a MySQL driver.
	// This is a basic check - the service is running and accepting connections.
	if buf[4] == 0xff {
		return false
	}
	return false // conservative: just verify service is up, don't try raw protocol auth
}

func checkRedis(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	if pass == "" {
		fmt.Fprintf(conn, "PING\r\n")
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		return n > 0 && strings.Contains(string(buf[:n]), "+PONG")
	}

	fmt.Fprintf(conn, "AUTH %s\r\n", pass)
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	return n > 0 && strings.Contains(string(buf[:n]), "+OK")
}

func checkMongoDB(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	// MongoDB wire protocol: try unauthenticated ismaster command
	// If it responds without error, no auth is required
	ismaster := []byte{
		0x3f, 0x00, 0x00, 0x00, // message length
		0x01, 0x00, 0x00, 0x00, // request id
		0x00, 0x00, 0x00, 0x00, // response to
		0xd4, 0x07, 0x00, 0x00, // opcode: OP_QUERY
		0x00, 0x00, 0x00, 0x00, // flags
		0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, // "admin."
		0x24, 0x63, 0x6d, 0x64, 0x00, // "$cmd"
		0x00, 0x00, 0x00, 0x00, // skip
		0x01, 0x00, 0x00, 0x00, // return
		0x13, 0x00, 0x00, 0x00, // doc length
		0x01,                   // type: double
		0x69, 0x73, 0x6d, 0x61, 0x73, 0x74, 0x65, 0x72, 0x00, // "ismaster"
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, // 1.0
		0x00, // end doc
	}
	conn.Write(ismaster)
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil || n < 36 {
		return false
	}
	// If we get a response and pass is empty, it means no auth required
	return pass == "" && n > 36
}

func checkPostgreSQL(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	// PostgreSQL startup message
	userBytes := []byte(user)
	dbBytes := []byte(user) // default db = user
	msgBody := append([]byte{0x00, 0x03, 0x00, 0x00}, []byte("user\x00")...)
	msgBody = append(msgBody, userBytes...)
	msgBody = append(msgBody, 0x00)
	msgBody = append(msgBody, []byte("database\x00")...)
	msgBody = append(msgBody, dbBytes...)
	msgBody = append(msgBody, 0x00, 0x00)

	length := uint32(len(msgBody) + 4)
	msg := []byte{byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)}
	msg = append(msg, msgBody...)
	conn.Write(msg)

	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)
	if n < 1 {
		return false
	}

	// 'R' = auth request, check if auth type is 0 (ok, no password needed)
	if buf[0] == 'R' && n >= 9 && buf[8] == 0 {
		return true
	}

	// For password auth, we'd need to handle md5 or cleartext
	if buf[0] == 'R' && n >= 9 && (buf[8] == 3 || buf[8] == 5) {
		if buf[8] == 3 {
			// cleartext password
			passMsg := append([]byte{'p'}, make([]byte, 4)...)
			passBytes := append([]byte(pass), 0x00)
			pLen := uint32(len(passBytes) + 4)
			passMsg[1] = byte(pLen >> 24)
			passMsg[2] = byte(pLen >> 16)
			passMsg[3] = byte(pLen >> 8)
			passMsg[4] = byte(pLen)
			passMsg = append(passMsg, passBytes...)
			conn.Write(passMsg)

			n2, _ := conn.Read(buf)
			return n2 > 0 && buf[0] == 'R' && n2 >= 9 && buf[8] == 0
		}
	}

	return false
}

func checkMSSQL(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	conn.Close()
	return false
}
