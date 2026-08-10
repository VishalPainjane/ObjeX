package s3

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

const MaxBatchDeleteKeys = 1000

// DeleteObjectError is one per-key failure in a batch delete response.
type DeleteObjectError struct {
	Key     string
	Code    string
	Message string
}

// ParseDeleteObjectKeys extracts object keys from a DeleteObjects XML body.
// Any descendant <Key> element is accepted (matches V1 behavior).
func ParseDeleteObjectKeys(body []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var keys []string
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "Key" {
			continue
		}
		var key string
		if err := decoder.DecodeElement(&key, &se); err != nil {
			return nil, err
		}
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// WriteDeleteObjectsResult writes S3 DeleteResult XML.
func WriteDeleteObjectsResult(w http.ResponseWriter, deleted []string, errs []DeleteObjectError) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<DeleteResult xmlns="%s">`+"\n", xmlNS)
	for _, key := range deleted {
		b.WriteString("  <Deleted>\n")
		fmt.Fprintf(&b, "    <Key>%s</Key>\n", escapeXML(key))
		b.WriteString("  </Deleted>\n")
	}
	for _, e := range errs {
		b.WriteString("  <Error>\n")
		fmt.Fprintf(&b, "    <Key>%s</Key>\n", escapeXML(e.Key))
		fmt.Fprintf(&b, "    <Code>%s</Code>\n", escapeXML(e.Code))
		fmt.Fprintf(&b, "    <Message>%s</Message>\n", escapeXML(e.Message))
		b.WriteString("  </Error>\n")
	}
	b.WriteString("</DeleteResult>\n")
	_, _ = w.Write(b.Bytes())
}
