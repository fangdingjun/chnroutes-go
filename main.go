package main

import (
	"bufio"
	"fmt"
	"github.com/EvilSuperstars/go-cidrman"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	ipdataurl = "http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest"
)

func getData(savefile string) error {
	resp, err := http.Get(ipdataurl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http error %d", resp.StatusCode)
	}

	fp, err := os.Create(savefile)
	if err != nil {
		return err
	}
	defer fp.Close()

	io.Copy(fp, resp.Body)

	return nil
}

func numToMask(n int) int {
	n1 := math.Log2(float64(n))
	return 32 - int(n1)
}

func parseLine(l string) *net.IPNet {
	ll := strings.Split(l, "|")
	ipString := ll[3]
	numString := ll[4]

	ip := net.ParseIP(ipString)
	n, err := strconv.Atoi(numString)
	if ip == nil || err != nil {
		return nil
	}

	prefixLen := numToMask(n)
	mask := net.CIDRMask(prefixLen, 32)

	return &net.IPNet{IP: ip, Mask: mask}
}

func parseData(fn string) []*net.IPNet {
	fp, err := os.Open(fn)
	if err != nil {
		log.Println(err)
		return nil
	}
	defer fp.Close()

	ipnets := []*net.IPNet{}

	r := bufio.NewReader(fp)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Println(err)
			}
			break
		}

		if line[0] == '#' {
			continue
		}
		if strings.HasPrefix(line, "apnic|CN|ipv4") {
			ipnet := parseLine(line)
			if ipnet != nil {
				ipnets = append(ipnets, ipnet)
			}
		}
	}
	return ipnets
}

func writeRouteScriptLinux(ipnet []*net.IPNet) error {
	fp, err := os.Create("routes_add.sh")
	if err != nil {
		return err
	}
	defer fp.Close()

	fp1, err := os.Create("routes_del.sh")
	if err != nil {
		return err
	}
	defer fp1.Close()

	fmt.Fprintf(fp, `#!/bin/bash
# route has two formats
#   1.2.3.4 dev ppp0  src 10.150.16.112
#   1.2.3.4 via 192.168.1.1 dev eth0 src 192.168.1.160 uid 1000
gw=$(ip route get 1.2.3.4 | awk '/1.2.3.4/{gsub("1.2.3.4", "");gsub("src.*", "");print}')
ip -batch - <<EOF
`)

	fmt.Fprintf(fp1, "#!/bin/bash\n")
	fmt.Fprintf(fp1, "ip -batch - <<EOF\n")

	for _, n := range ipnet {
		m, _ := n.Mask.Size()
		fmt.Fprintf(fp, "route add %s/%d $gw\n", n.IP.String(), m)
		fmt.Fprintf(fp1, "route del %s/%d\n", n.IP.String(), m)
	}

	fmt.Fprintf(fp, "EOF\n")
	fmt.Fprintf(fp1, "EOF\n")

	return nil
}

func writeIPNet(ipnet []*net.IPNet) error {
	fp, err := os.Create("ipnet_cn.txt")
	if err != nil {
		return err
	}
	defer fp.Close()

	for _, n := range ipnet {
		m, _ := n.Mask.Size()
		fmt.Fprintf(fp, "%s/%d\n", n.IP.String(), m)
	}
	return nil
}

func main() {
	ipDataFile := "ip.txt"
	if _, err := os.Stat(ipDataFile); err != nil {
		if err1 := getData(ipDataFile); err1 != nil {
			log.Println(err)
			os.Exit(1)
		}
	}

	ipnet := parseData(ipDataFile)

	mergedIPNet, err := cidrman.MergeIPNets(ipnet)
	if err != nil {
		log.Fatal(err)
	}

	writeIPNet(mergedIPNet)
	writeRouteScriptLinux(mergedIPNet)
}
