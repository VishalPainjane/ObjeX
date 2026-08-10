package s3

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/VishalPainjane/objex/internal/metadata"
)

const xmlNS = "http://s3.amazonaws.com/doc/2006-03-01/"

func escapeXML(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// WriteError writes S3 XML error to w.
func WriteError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>%s</Code>
  <Message>%s</Message>
</Error>`, escapeXML(code), escapeXML(message))
}

// WriteListBuckets writes ListAllMyBucketsResult XML.
func WriteListBuckets(w http.ResponseWriter, buckets []metadata.Bucket) {
	w.Header().Set("Content-Type", "application/xml")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<ListAllMyBucketsResult xmlns="` + xmlNS + `">` + "\n")
	b.WriteString(`  <Owner><ID>owner</ID><DisplayName>owner</DisplayName></Owner>` + "\n")
	b.WriteString(`  <Buckets>` + "\n")
	for _, bucket := range buckets {
		b.WriteString(`    <Bucket>` + "\n")
		fmt.Fprintf(&b, `      <Name>%s</Name>`+"\n", escapeXML(bucket.Name))
		fmt.Fprintf(&b, `      <CreationDate>%s</CreationDate>`+"\n", formatS3Time(bucket.CreatedAt))
		b.WriteString(`    </Bucket>` + "\n")
	}
	b.WriteString(`  </Buckets>` + "\n")
	b.WriteString(`</ListAllMyBucketsResult>` + "\n")
	w.Write([]byte(b.String()))
}

// ListObjectsParams for XML list output.
type ListObjectsParams struct {
	Bucket            string
	Prefix            string
	Delimiter         string
	ListType2         bool
	ContinuationToken string
	StartAfter        string
	Result            metadata.ListResult
}

// WriteListObjects writes ListBucketResult XML (v1 or v2).
func WriteListObjects(w http.ResponseWriter, p ListObjectsParams) {
	w.Header().Set("Content-Type", "application/xml")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<ListBucketResult xmlns="` + xmlNS + `">` + "\n")
	fmt.Fprintf(&b, `  <Name>%s</Name>`+"\n", escapeXML(p.Bucket))
	fmt.Fprintf(&b, `  <Prefix>%s</Prefix>`+"\n", escapeXML(p.Prefix))
	if p.Delimiter != "" {
		fmt.Fprintf(&b, `  <Delimiter>%s</Delimiter>`+"\n", escapeXML(p.Delimiter))
	}
	if p.ListType2 {
		fmt.Fprintf(&b, `  <KeyCount>%d</KeyCount>`+"\n", len(p.Result.Objects))
	}
	fmt.Fprintf(&b, `  <IsTruncated>%t</IsTruncated>`+"\n", p.Result.IsTruncated)
	if p.ListType2 && p.ContinuationToken != "" {
		fmt.Fprintf(&b, `  <ContinuationToken>%s</ContinuationToken>`+"\n", escapeXML(p.ContinuationToken))
	}
	if p.ListType2 && p.StartAfter != "" {
		fmt.Fprintf(&b, `  <StartAfter>%s</StartAfter>`+"\n", escapeXML(p.StartAfter))
	}
	if p.ListType2 && p.Result.IsTruncated && p.Result.NextContinuationToken != "" {
		fmt.Fprintf(&b, `  <NextContinuationToken>%s</NextContinuationToken>`+"\n", escapeXML(p.Result.NextContinuationToken))
	}
	for _, obj := range p.Result.Objects {
		if strings.HasSuffix(obj.Key, "/") {
			continue
		}
		b.WriteString(`  <Contents>` + "\n")
		fmt.Fprintf(&b, `    <Key>%s</Key>`+"\n", escapeXML(obj.Key))
		fmt.Fprintf(&b, `    <LastModified>%s</LastModified>`+"\n", formatS3Time(obj.UpdatedAt))
		fmt.Fprintf(&b, `    <ETag>&quot;%s&quot;</ETag>`+"\n", escapeXML(obj.ETag))
		fmt.Fprintf(&b, `    <Size>%d</Size>`+"\n", obj.Size)
		b.WriteString(`    <StorageClass>STANDARD</StorageClass>` + "\n")
		b.WriteString(`  </Contents>` + "\n")
	}
	for _, cp := range p.Result.CommonPrefixes {
		b.WriteString(`  <CommonPrefixes>` + "\n")
		fmt.Fprintf(&b, `    <Prefix>%s</Prefix>`+"\n", escapeXML(cp))
		b.WriteString(`  </CommonPrefixes>` + "\n")
	}
	b.WriteString(`</ListBucketResult>` + "\n")
	w.Write([]byte(b.String()))
}

// WriteBucketLocation writes GetBucketLocation XML.
func WriteBucketLocation(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<LocationConstraint xmlns="` + xmlNS + `">us-east-1</LocationConstraint>`))
}

func formatS3Time(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// WriteInitiateMultipartUpload writes InitiateMultipartUploadResult XML.
func WriteInitiateMultipartUpload(w http.ResponseWriter, bucket, key, uploadID string) {
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<InitiateMultipartUploadResult xmlns="%s">
  <Bucket>%s</Bucket>
  <Key>%s</Key>
  <UploadId>%s</UploadId>
</InitiateMultipartUploadResult>`, xmlNS, escapeXML(bucket), escapeXML(key), escapeXML(uploadID))
}

// WriteCompleteMultipartUpload writes CompleteMultipartUploadResult XML.
func WriteCompleteMultipartUpload(w http.ResponseWriter, bucket, key, location, etag string) {
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUploadResult xmlns="%s">
  <Location>%s</Location>
  <Bucket>%s</Bucket>
  <Key>%s</Key>
  <ETag>&quot;%s&quot;</ETag>
</CompleteMultipartUploadResult>`, xmlNS, escapeXML(location), escapeXML(bucket), escapeXML(key), escapeXML(etag))
}

// WriteListParts writes ListPartsResult XML.
func WriteListParts(w http.ResponseWriter, bucket, key, uploadID string, parts []metadata.MultipartPart) {
	w.Header().Set("Content-Type", "application/xml")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<ListPartsResult xmlns="` + xmlNS + `">` + "\n")
	fmt.Fprintf(&b, `  <Bucket>%s</Bucket>`+"\n", escapeXML(bucket))
	fmt.Fprintf(&b, `  <Key>%s</Key>`+"\n", escapeXML(key))
	fmt.Fprintf(&b, `  <UploadId>%s</UploadId>`+"\n", escapeXML(uploadID))
	b.WriteString(`  <IsTruncated>false</IsTruncated>` + "\n")
	for _, p := range parts {
		b.WriteString(`  <Part>` + "\n")
		fmt.Fprintf(&b, `    <PartNumber>%d</PartNumber>`+"\n", p.PartNumber)
		fmt.Fprintf(&b, `    <LastModified>%s</LastModified>`+"\n", formatS3Time(p.UpdatedAt))
		fmt.Fprintf(&b, `    <ETag>&quot;%s&quot;</ETag>`+"\n", escapeXML(p.ETag))
		fmt.Fprintf(&b, `    <Size>%d</Size>`+"\n", p.Size)
		b.WriteString(`  </Part>` + "\n")
	}
	b.WriteString(`</ListPartsResult>` + "\n")
	w.Write([]byte(b.String()))
}

// WriteListMultipartUploads writes ListMultipartUploadsResult XML.
func WriteListMultipartUploads(w http.ResponseWriter, bucket string, uploads []metadata.MultipartUpload) {
	w.Header().Set("Content-Type", "application/xml")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<ListMultipartUploadsResult xmlns="` + xmlNS + `">` + "\n")
	fmt.Fprintf(&b, `  <Bucket>%s</Bucket>`+"\n", escapeXML(bucket))
	b.WriteString(`  <IsTruncated>false</IsTruncated>` + "\n")
	for _, u := range uploads {
		b.WriteString(`  <Upload>` + "\n")
		fmt.Fprintf(&b, `    <Key>%s</Key>`+"\n", escapeXML(u.Key))
		fmt.Fprintf(&b, `    <UploadId>%s</UploadId>`+"\n", escapeXML(u.ID))
		fmt.Fprintf(&b, `    <Initiated>%s</Initiated>`+"\n", formatS3Time(u.CreatedAt))
		b.WriteString(`  </Upload>` + "\n")
	}
	b.WriteString(`</ListMultipartUploadsResult>` + "\n")
	w.Write([]byte(b.String()))
}

// WriteCopyObjectResult writes CopyObjectResult XML.
func WriteCopyObjectResult(w http.ResponseWriter, etag string, lastModified time.Time) {
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<CopyObjectResult xmlns="%s">
  <LastModified>%s</LastModified>
  <ETag>&quot;%s&quot;</ETag>
</CopyObjectResult>`, xmlNS, formatS3Time(lastModified), escapeXML(etag))
}
