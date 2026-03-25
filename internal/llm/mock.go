package llm

import "context"

type MockClient struct {
	NameValue  string
	ModelValue string
	Handler    func(req Request) (Response, error)
}

func (m MockClient) Name() string {
	if m.NameValue == "" {
		return "mock"
	}
	return m.NameValue
}

func (m MockClient) Model() string {
	if m.ModelValue == "" {
		return "mock-model"
	}
	return m.ModelValue
}

func (m MockClient) Generate(_ context.Context, req Request) (Response, error) {
	if m.Handler != nil {
		return m.Handler(req)
	}
	return Response{Text: "{}"}, nil
}
