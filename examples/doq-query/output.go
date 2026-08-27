package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	dnscodec "goark.dev/gnalloy/codec/dns"
)

var errInvalidQueryType = errors.New("gnalloy/examples/doq-query: invalid query type")

func parseQueryType(text string) (uint16, error) {
	switch strings.ToUpper(strings.TrimSpace(text)) {
	case "A":
		return dnscodec.TypeA, nil
	case "AAAA":
		return dnscodec.TypeAAAA, nil
	case "CNAME":
		return dnscodec.TypeCNAME, nil
	case "MX":
		return dnscodec.TypeMX, nil
	case "NS":
		return dnscodec.TypeNS, nil
	case "PTR":
		return dnscodec.TypePTR, nil
	case "SOA":
		return dnscodec.TypeSOA, nil
	case "SRV":
		return dnscodec.TypeSRV, nil
	case "TXT":
		return dnscodec.TypeTXT, nil
	default:
		return 0, fmt.Errorf("%w: %s", errInvalidQueryType, text)
	}
}

func printMessage(w io.Writer, msg dnscodec.Message) {
	fmt.Fprintf(w, "id=%d rcode=%d answers=%d authorities=%d additionals=%d\n",
		msg.ID, msg.ResponseCode, len(msg.Answers), len(msg.Authorities), len(msg.Additionals))
	for _, answer := range msg.Answers {
		fmt.Fprintf(w, "answer %s %s ttl=%d %s\n",
			answer.Name, typeName(answer.Type), answer.TTL, resourceValue(answer))
	}
}

func resourceValue(resource dnscodec.Resource) string {
	if ip := resource.IP(); ip != nil {
		return ip.String()
	}
	if target, ok := resource.Target(); ok {
		return target
	}
	if mx, ok := resource.MX(); ok {
		return fmt.Sprintf("preference=%d exchange=%s", mx.Preference, mx.Exchange)
	}
	if srv, ok := resource.SRV(); ok {
		return fmt.Sprintf("priority=%d weight=%d port=%d target=%s", srv.Priority, srv.Weight, srv.Port, srv.Target)
	}
	if soa, ok := resource.SOA(); ok {
		return fmt.Sprintf("mname=%s rname=%s serial=%d refresh=%d retry=%d expire=%d minimum=%d",
			soa.MName, soa.RName, soa.Serial, soa.Refresh, soa.Retry, soa.Expire, soa.Minimum)
	}
	if txt, ok := resource.TXT(); ok {
		return strings.Join(txt, " ")
	}
	return fmt.Sprintf("%x", resource.Data)
}

func typeName(qtype uint16) string {
	switch qtype {
	case dnscodec.TypeA:
		return "A"
	case dnscodec.TypeAAAA:
		return "AAAA"
	case dnscodec.TypeCNAME:
		return "CNAME"
	case dnscodec.TypeMX:
		return "MX"
	case dnscodec.TypeNS:
		return "NS"
	case dnscodec.TypePTR:
		return "PTR"
	case dnscodec.TypeSOA:
		return "SOA"
	case dnscodec.TypeSRV:
		return "SRV"
	case dnscodec.TypeTXT:
		return "TXT"
	default:
		return fmt.Sprintf("TYPE%d", qtype)
	}
}
