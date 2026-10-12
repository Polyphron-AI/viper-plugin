package kit

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Polyphron-AI/viper-plugin/proto"
)

// server turns gRPC calls from Viper into the Plugin's handlers.
type server struct {
	pb.UnimplementedPluginServer
	p    *Plugin
	host pb.HostClient
}

// call wraps a request's Call for a handler.
func (s *server) call(ctx context.Context, c *pb.Call) *Call {
	if c == nil {
		c = &pb.Call{}
	}

	return &Call{Context: ctx, Call: c, Host: s.host}
}

// Describe returns the plugin's description.
func (s *server) Describe(context.Context, *pb.DescribeRequest) (*pb.Description, error) {
	return s.p.Description, nil
}

// RunBlock runs a declared block.
func (s *server) RunBlock(ctx context.Context, r *pb.BlockRequest) (*pb.BlockReply, error) {
	run, ok := s.p.Blocks[r.Block]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no block %q", r.Block)
	}

	cfg := map[string]any{}
	if len(r.Config) > 0 {
		if err := json.Unmarshal(r.Config, &cfg); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	out, way, err := run(s.call(ctx, r.Call), cfg)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}

	return &pb.BlockReply{Output: raw, Way: way}, nil
}

// CallTool runs a declared tool. A handler error goes back to the agent as
// an error result, not a failed call.
func (s *server) CallTool(ctx context.Context, r *pb.ToolRequest) (*pb.ToolReply, error) {
	run, ok := s.p.Tools[r.Tool]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no tool %q", r.Tool)
	}

	args := map[string]any{}
	if len(r.Arguments) > 0 {
		if err := json.Unmarshal(r.Arguments, &args); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	out, err := run(s.call(ctx, r.Call), args)
	if err != nil {
		raw, _ := json.Marshal(err.Error())

		return &pb.ToolReply{Result: raw, IsError: true}, nil
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}

	return &pb.ToolReply{Result: raw}, nil
}

// HandleRoute answers a declared route.
func (s *server) HandleRoute(ctx context.Context, r *pb.RouteRequest) (*pb.RouteReply, error) {
	run, ok := s.p.Routes[r.Method+" "+r.Route]
	if !ok {
		return &pb.RouteReply{Status: 404}, nil
	}

	return run(s.call(ctx, r.Call), r)
}

// HandleEvent passes a record event on.
func (s *server) HandleEvent(ctx context.Context, r *pb.EventRequest) (*pb.Done, error) {
	if s.p.Event == nil {
		return &pb.Done{}, nil
	}

	return &pb.Done{}, s.p.Event(s.call(ctx, r.Call), r)
}

// RunJob runs a queued job.
func (s *server) RunJob(ctx context.Context, r *pb.JobRequest) (*pb.Done, error) {
	run, ok := s.p.Jobs[r.Kind]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no job kind %q", r.Kind)
	}

	return &pb.Done{}, run(s.call(ctx, r.Call), r.Payload)
}
