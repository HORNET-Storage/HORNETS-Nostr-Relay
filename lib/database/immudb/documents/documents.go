// Package immudoc provides a lightweight SDK helper for the immudb document API
package documents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const baseURL = "http://localhost:8080/api/v2"

// Client represents an immudb document API client
type Client struct {
	sessionID string
	baseURL   string
	client    *http.Client
}

// NewClient creates a new document API client
func NewClient(sessionID string) *Client {
	return &Client{
		sessionID: sessionID,
		baseURL:   baseURL,
		client:    &http.Client{},
	}
}

// doRequest is a helper function to make HTTP requests
func (c *Client) doRequest(method, endpoint string, body interface{}) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("error marshaling request body: %v", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, c.baseURL+endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("sessionID", c.sessionID)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		var errResp struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, errResp.Message)
	}

	return resp, nil
}

// Collection represents a document collection
type Collection struct {
	Name    string   `json:"name"`
	Fields  []*Field `json:"fields,omitempty"`
	Indexes []*Index `json:"indexes,omitempty"`
}

// Field represents a collection field
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Index represents a collection index
type Index struct {
	Fields   []string `json:"fields"`
	IsUnique bool     `json:"unique"`
}

// CreateCollection creates a new document collection
func (c *Client) CreateCollection(ctx context.Context, collection *Collection) error {
	resp, err := c.doRequest(http.MethodPost, fmt.Sprintf("/collection/%s", collection.Name), collection)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetCollection retrieves a collection's metadata
func (c *Client) GetCollection(ctx context.Context, name string) (*Collection, error) {
	resp, err := c.doRequest(http.MethodGet, fmt.Sprintf("/collection/%s", name), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var collection Collection
	if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	return &collection, nil
}

// DeleteCollection deletes a collection
func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	resp, err := c.doRequest(http.MethodDelete, fmt.Sprintf("/collection/%s", name), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Document represents a generic document with arbitrary fields
type Document map[string]interface{}

// InsertDocumentsRequest represents a request to insert documents
type InsertDocumentsRequest struct {
	Documents []Document `json:"documents"`
}

// InsertDocumentsResponse represents the response from inserting documents
type InsertDocumentsResponse struct {
	DocumentIDs []string `json:"documentIds"`
	TxID        uint64   `json:"txId"`
}

// InsertDocuments inserts one or more documents into a collection
func (c *Client) InsertDocuments(ctx context.Context, collectionName string, documents []Document) (*InsertDocumentsResponse, error) {
	req := InsertDocumentsRequest{Documents: documents}
	resp, err := c.doRequest(http.MethodPost, fmt.Sprintf("/collection/%s/documents", collectionName), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result InsertDocumentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}
	return &result, nil
}

// SearchQuery represents a document search query
type SearchQuery struct {
	CollectionName string           `json:"collectionName,omitempty"`
	Query          *QueryExpression `json:"query,omitempty"`
	Page           int              `json:"page"`
	PerPage        int              `json:"pageSize"`
}

// QueryExpression represents a search query expression
type QueryExpression struct {
	Expressions []Expression `json:"expressions,omitempty"`
	Limit       int          `json:"limit,omitempty"`
}

// Expression represents a query expression
type Expression struct {
	FieldComparisons []FieldComparison `json:"fieldComparisons,omitempty"`
}

// FieldComparison represents a field comparison in a query
type FieldComparison struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// SearchResult represents a document search result
type SearchResult struct {
	Document      Document `json:"document"`
	TransactionID uint64   `json:"transactionId"`
}

// SearchDocumentsResponse represents the response from a document search
type SearchDocumentsResponse struct {
	Revisions []SearchResult `json:"revisions"`
	Page      int            `json:"page"`
	PerPage   int            `json:"perPage"`
	Total     int            `json:"total"`
}

// SearchDocuments searches for documents in a collection
func (c *Client) SearchDocuments(ctx context.Context, collectionName string, query *SearchQuery) (*SearchDocumentsResponse, error) {
	resp, err := c.doRequest(http.MethodPost, fmt.Sprintf("/collection/%s/documents/search", collectionName), query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result SearchDocumentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}
	return &result, nil
}

// DeleteDocumentsRequest represents a request to delete documents
type DeleteDocumentsRequest struct {
	Query *QueryExpression `json:"query"`
}

// DeleteDocumentsResponse represents the response from deleting documents
type DeleteDocumentsResponse struct {
	DocumentIDs []string `json:"documentIds"`
}

// DeleteDocuments deletes documents matching the given query
func (c *Client) DeleteDocuments(ctx context.Context, collectionName string, query *QueryExpression) (*DeleteDocumentsResponse, error) {
	req := DeleteDocumentsRequest{Query: query}
	resp, err := c.doRequest(http.MethodPost, fmt.Sprintf("/collection/%s/documents/delete", collectionName), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result DeleteDocumentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}
	return &result, nil
}

// ReplaceDocumentsRequest represents a request to replace documents
type ReplaceDocumentsRequest struct {
	Query    *QueryExpression `json:"query"`
	Document Document         `json:"document"`
}

// ReplaceDocumentsResponse represents the response from replacing documents
type ReplaceDocumentsResponse struct {
	DocumentIDs []string `json:"documentIds"`
}

// ReplaceDocuments replaces documents matching the query with the new document
func (c *Client) ReplaceDocuments(ctx context.Context, collectionName string, query *QueryExpression, newDocument Document) (*ReplaceDocumentsResponse, error) {
	req := ReplaceDocumentsRequest{
		Query:    query,
		Document: newDocument,
	}
	resp, err := c.doRequest(http.MethodPut, fmt.Sprintf("/collection/%s/documents/replace", collectionName), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ReplaceDocumentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}
	return &result, nil
}

// CountDocumentsRequest represents a request to count documents
type CountDocumentsRequest struct {
	Query *QueryExpression `json:"query"`
}

// CountDocumentsResponse represents the response from counting documents
type CountDocumentsResponse struct {
	Count int `json:"count"`
}

// CountDocuments counts documents matching the given query
func (c *Client) CountDocuments(ctx context.Context, collectionName string, query *QueryExpression) (*CountDocumentsResponse, error) {
	req := CountDocumentsRequest{Query: query}
	resp, err := c.doRequest(http.MethodPost, fmt.Sprintf("/collection/%s/documents/count", collectionName), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result CountDocumentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}
	return &result, nil
}

// CreateIndexRequest represents a request to create an index
type CreateIndexRequest struct {
	Fields   []string `json:"fields"`
	IsUnique bool     `json:"unique"`
}

// CreateIndexResponse represents the response from creating an index
type CreateIndexResponse struct {
	Name     string   `json:"name"`
	Fields   []string `json:"fields"`
	IsUnique bool     `json:"unique"`
}

// CreateIndex creates a new index on the specified fields in a collection
func (c *Client) CreateIndex(ctx context.Context, collectionName string, fields []string, isUnique bool) (*CreateIndexResponse, error) {
	req := CreateIndexRequest{
		Fields:   fields,
		IsUnique: isUnique,
	}

	resp, err := c.doRequest(http.MethodPost, fmt.Sprintf("/collection/%s/index", collectionName), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result CreateIndexResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}
	return &result, nil
}

// DeleteIndexResponse represents the response from deleting an index
type DeleteIndexResponse struct {
	Status string `json:"status,omitempty"`
}

// DeleteIndex deletes an index from a collection
func (c *Client) DeleteIndex(ctx context.Context, collectionName string, fields []string) error {
	// Convert fields array to comma-separated string
	fieldsStr := ""
	for i, field := range fields {
		if i > 0 {
			fieldsStr += ","
		}
		fieldsStr += field
	}

	resp, err := c.doRequest(http.MethodDelete, fmt.Sprintf("/collection/%s/index?fields=%s", collectionName, fieldsStr), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result DeleteIndexResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("error decoding response: %v", err)
	}

	return nil
}

// ListIndexes returns all indexes for a collection
// Note: This is retrieved through the GetCollection API since indexes are part of collection metadata
func (c *Client) ListIndexes(ctx context.Context, collectionName string) ([]*Index, error) {
	collection, err := c.GetCollection(ctx, collectionName)
	if err != nil {
		return nil, err
	}

	return collection.Indexes, nil
}

// AddFieldRequest represents a request to add a field to a collection
type AddFieldRequest struct {
	Field Field `json:"field"`
}

// AddFieldResponse represents the response from adding a field
type AddFieldResponse struct {
	Status string `json:"status,omitempty"`
}

// FieldType represents the valid types for collection fields
type FieldType string

const (
	FieldTypeString  FieldType = "STRING"
	FieldTypeInteger FieldType = "INTEGER"
	FieldTypeBoolean FieldType = "BOOLEAN"
	FieldTypeDouble  FieldType = "DOUBLE"
	FieldTypeUUID    FieldType = "UUID"
)

// AddField adds a new field to a collection
func (c *Client) AddField(ctx context.Context, collectionName string, fieldName string, fieldType FieldType) error {
	req := AddFieldRequest{
		Field: Field{
			Name: fieldName,
			Type: string(fieldType),
		},
	}

	resp, err := c.doRequest(http.MethodPost, fmt.Sprintf("/collection/%s/field", collectionName), req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result AddFieldResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("error decoding response: %v", err)
	}

	return nil
}

// RemoveFieldResponse represents the response from removing a field
type RemoveFieldResponse struct {
	Status string `json:"status,omitempty"`
}

// RemoveField removes a field from a collection
func (c *Client) RemoveField(ctx context.Context, collectionName string, fieldName string) error {
	resp, err := c.doRequest(http.MethodDelete, fmt.Sprintf("/collection/%s/field/%s", collectionName, fieldName), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result RemoveFieldResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("error decoding response: %v", err)
	}

	return nil
}

// ListFields returns all fields for a collection
// Note: This is retrieved through the GetCollection API since fields are part of collection metadata
func (c *Client) ListFields(ctx context.Context, collectionName string) ([]*Field, error) {
	collection, err := c.GetCollection(ctx, collectionName)
	if err != nil {
		return nil, err
	}

	return collection.Fields, nil
}
