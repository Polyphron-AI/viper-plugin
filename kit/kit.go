// Package kit is the Go helper for writing a Viper plugin (D-023). A plugin
// is a main package that fills a Plugin and calls Serve. Plugins in other
// languages follow plugins/proto/plugin.proto directly.
package kit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/Polyphron-AI/viper-plugin/proto"
)

// Handshake is the first word of the line a plugin prints once it serves.
const Handshake = "VIPER-PLUGIN"

// Protocol is the handshake version this kit speaks.
const Protocol = 1

// Env names Viper sets when it starts a plugin.
const (
	EnvToken = "VIPER_PLUGIN_TOKEN"
	EnvHost  = "VIPER_PLUGIN_HOST"
)

// BlockFunc runs one workflow step. It returns the step's output and the way
// out ("" for the default).
type BlockFunc func(c *Call, config map[string]any) (any, string, error)

// ToolFunc runs one MCP tool and returns its result.
type ToolFunc func(c *Call, args map[string]any) (any, error)

// RouteFunc answers one HTTP request.
type RouteFunc func(c *Call, r *pb.RouteRequest) (*pb.RouteReply, error)

// EventFunc handles one record event.
type EventFunc func(c *Call, e *pb.EventRequest) error

// JobFunc runs one queued job.
type JobFunc func(c *Call, payload []byte) error

// Plugin is everything a plugin declares and does. Blocks, Tools and Jobs are
// keyed by name; Routes by "METHOD path", like "GET items/:id".
type Plugin struct {
	Description *pb.Description
	Blocks      map[string]BlockFunc
	Tools       map[string]ToolFunc
	Routes      map[string]RouteFunc
	Event       EventFunc
	Jobs        map[string]JobFunc
}

// Call is one call from Viper. Host requests made through it act in the
// call's project as the call's caller, or as the service on an elevated
// route (c.Elevated, within the manifest's grant).
type Call struct {
	context.Context
	*pb.Call
	Host pb.HostClient
}

// Settings decodes the project's settings for this plugin into v.
func (c *Call) Settings(v any) error {
	if len(c.Call.Settings) == 0 {
		return nil
	}

	return json.Unmarshal(c.Call.Settings, v)
}

// List reads records from a collection with an optional filter.
func (c *Call) List(collection, filter string) ([]map[string]any, error) {
	out, err := c.Host.ListRecords(
		c,
		&pb.ListRecordsRequest{Call: c.Id, Collection: collection, Filter: filter},
	)
	if err != nil {
		return nil, err
	}

	var items []map[string]any
	err = json.Unmarshal(out.Items, &items)

	return items, err
}

// Create adds a record to a collection and returns it.
func (c *Call) Create(collection string, body map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	out, err := c.Host.CreateRecord(
		c,
		&pb.WriteRecordRequest{Call: c.Id, Collection: collection, Body: raw},
	)
	if err != nil {
		return nil, err
	}

	var rec map[string]any
	err = json.Unmarshal(out.Record, &rec)

	return rec, err
}

// CreateUpload opens an upload session for one file on a record's file
// field. Fill in the request's file fields; the call is set here. Hand the
// reply's Url (and Headers) to the visitor, or proxy parts when Mode is
// "parts".
func (c *Call) CreateUpload(r *pb.CreateUploadRequest) (*pb.CreateUploadReply, error) {
	r.Call = c.Id

	return c.Host.CreateUpload(c, r)
}

// CompleteUpload finishes a session this plugin started and returns the
// record with the file added. sha256 is optional.
func (c *Call) CompleteUpload(upload, sha256 string) (map[string]any, error) {
	out, err := c.Host.CompleteUpload(
		c,
		&pb.CompleteUploadRequest{Call: c.Id, Upload: upload, Sha256: sha256},
	)
	if err != nil {
		return nil, err
	}

	var rec map[string]any
	err = json.Unmarshal(out.Record, &rec)

	return rec, err
}

// AccessToken returns user's token at a connected account (like
// "google") for scopes, and when it expires. The manifest's connections
// and the project's grant must name them. A user's call may ask only for
// that user; a job or event (with background) only for a user its payload
// names.
func (c *Call) AccessToken(user, provider string, scopes ...string) (string, time.Time, error) {
	out, err := c.Host.GetAccessToken(
		c,
		&pb.AccessTokenRequest{Call: c.Id, User: user, Provider: provider, Scopes: scopes},
	)
	if err != nil {
		return "", time.Time{}, err
	}

	return out.Token, time.Unix(out.ExpiresUnix, 0), nil
}

// Log writes a line to Viper's log.
func (c *Call) Log(level, message string) {
	_, _ = c.Host.Log(
		c,
		&pb.LogRequest{Call: c.Id, Level: level, Message: message},
	)
}

// JSON is a RouteReply with a JSON body.
func JSON(status int, v any) (*pb.RouteReply, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return &pb.RouteReply{
		Status:  int32(status),
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
	}, nil
}

// Fail is an error Viper shows as is, with a 4xx status, instead of a generic
// plugin failure.
func Fail(message string) error {
	return status.Error(codes.InvalidArgument, message)
}

// Serve starts the plugin: it dials the host, serves the Plugin service on a
// loopback port, prints the handshake line and blocks until Viper closes its
// stdin (Viper stopped or went away).
func Serve(p *Plugin) error {
	token := os.Getenv(EnvToken)
	if token == "" {
		return errors.New("this is a Viper plugin; Viper starts it")
	}

	conn, err := grpc.NewClient(
		os.Getenv(EnvHost),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(withToken(token)),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(needToken(token)),
		grpc.MaxRecvMsgSize(64<<20),
	)
	pb.RegisterPluginServer(
		srv,
		&server{p: p, host: pb.NewHostClient(conn)},
	)

	go func() {
		_ = srv.Serve(lis)
	}()

	fmt.Printf("%s %d %s\n", Handshake, Protocol, lis.Addr().String())

	// Viper holds our stdin open while it wants us running. EOF means it
	// stopped us or died, so a plugin never outlives its Viper.
	_, _ = io.Copy(io.Discard, bufio.NewReader(os.Stdin))
	srv.GracefulStop()

	return nil
}

// withToken sends the token on every call to the host.
func withToken(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(
			ctx,
			"authorization",
			"Bearer "+token,
		)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// needToken refuses a call that doesn't carry the token, so no other local
// program can call the plugin.
func needToken(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		got := md.Get("authorization")

		if len(got) != 1 || strings.TrimPrefix(got[0], "Bearer ") != token {
			return nil, status.Error(codes.Unauthenticated, "wrong plugin token")
		}

		return handler(ctx, req)
	}
}
