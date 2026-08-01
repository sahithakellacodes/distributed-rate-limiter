package auth

type InMemoryAuthStore struct {
	clients map[string]string
}

func NewInMemoryAuthStore() *InMemoryAuthStore {
	return &InMemoryAuthStore{
		clients: map[string]string{
			"test-key-1": "client-1",
			"test-key-2": "client-2",
		},
	}
}

func (s *InMemoryAuthStore) GetClientID(apiKey string) (string, bool) {
	clientID, exists := s.clients[apiKey]
	return clientID, exists
}