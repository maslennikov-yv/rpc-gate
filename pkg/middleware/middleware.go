package middleware

import (
    "streaming-server/pkg/types"
)

// Chain represents a chain of middleware functions
type Chain struct {
    middlewares []types.Middleware
}

// NewChain creates a new middleware chain
func NewChain(middlewares ...types.Middleware) *Chain {
    return &Chain{
        middlewares: middlewares,
    }
}

// Add appends middleware to the chain
func (c *Chain) Add(middleware types.Middleware) *Chain {
    c.middlewares = append(c.middlewares, middleware)
    return c
}

// Execute executes the middleware chain with the final handler
func (c *Chain) Execute(req *types.JSONRPCRequest, ctx *types.RequestContext, finalHandler types.Handler) (*types.JSONRPCResponse, error) {
    if len(c.middlewares) == 0 {
        return finalHandler(req, ctx)
    }
    
    return c.executeMiddleware(0, req, ctx, finalHandler)
}

// executeMiddleware recursively executes middleware in the chain
func (c *Chain) executeMiddleware(index int, req *types.JSONRPCRequest, ctx *types.RequestContext, finalHandler types.Handler) (*types.JSONRPCResponse, error) {
    if index >= len(c.middlewares) {
        return finalHandler(req, ctx)
    }
    
    currentMiddleware := c.middlewares[index]
    nextHandler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
        return c.executeMiddleware(index+1, req, ctx, finalHandler)
    }
    
    return currentMiddleware(req, ctx, nextHandler)
}
