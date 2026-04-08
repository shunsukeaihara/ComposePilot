package httphandler

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/", s.handleHomePage)
	s.mux.HandleFunc("/projects/new", s.handleNewProjectPage)
	s.mux.HandleFunc("/projects/", s.handleProjectPages)
	s.mux.HandleFunc("/projects", s.handleCreateProjectPage)
	s.mux.HandleFunc("/settings/networks", s.handleNetworksPage)
	s.mux.HandleFunc("/settings/monitoring", s.handleMonitoringPage)
	s.mux.HandleFunc("/settings/monitoring/targets", s.handleNotificationTargets)
	s.mux.HandleFunc("/api/services", s.handleServices)
	s.mux.HandleFunc("/api/services/", s.handleServiceByID)
	s.mux.HandleFunc("/api/networks", s.handleNetworks)
}
