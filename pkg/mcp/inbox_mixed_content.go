package mcp

import (
	stdjson "encoding/json"
	"fmt"
)

// MCPMixedContent represents one content block of a tool which can return different kinds of blocks in one response (text, image or embedded resource)
type MCPMixedContent struct {
	Type     string
	Text     string
	MimeType string
	Data     string // Base64 encoded content for image blocks
	Resource any
}

// MarshalJSON serializes the block with the fields of its type, as MCPTextContent, MCPImageContent or MCPEmbeddedResource do
func (c MCPMixedContent) MarshalJSON() ([]byte, error) {
	switch c.Type {
	case "text":
		return stdjson.Marshal(&MCPTextContent{Type: c.Type, Text: c.Text})
	case "image":
		return stdjson.Marshal(&MCPImageContent{Type: c.Type, MimeType: c.MimeType, Data: c.Data})
	case "resource":
		return stdjson.Marshal(&MCPEmbeddedResource{Type: c.Type, Resource: c.Resource})
	}

	return nil, fmt.Errorf("unsupported mcp content type \"%s\"", c.Type)
}

// NewMCPMixedTextContent creates a text block
func NewMCPMixedTextContent(text string) *MCPMixedContent {
	return &MCPMixedContent{
		Type: "text",
		Text: text,
	}
}

// NewMCPMixedImageContent creates an image block from a content already encoded in base64
func NewMCPMixedImageContent(base64Data string, mimeType string) *MCPMixedContent {
	return &MCPMixedContent{
		Type:     "image",
		MimeType: mimeType,
		Data:     base64Data,
	}
}

// NewMCPMixedBlobResourceContent creates an embedded resource block with a binary content already encoded in base64
func NewMCPMixedBlobResourceContent(uri string, mimeType string, base64Data string) *MCPMixedContent {
	return &MCPMixedContent{
		Type: "resource",
		Resource: &MCPBlobResourceContents{
			URI:      uri,
			Blob:     base64Data,
			MimeType: mimeType,
		},
	}
}
