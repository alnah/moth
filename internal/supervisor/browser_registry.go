package supervisor

func (server *Server) registerBrowserHandlers() {
	server.handlers[MethodBrowserStart] = server.handleBrowserStart
	server.handlers[MethodBrowserStop] = server.handleBrowserStop
	server.handlers[MethodBrowserStatus] = server.handleBrowserStatus
	server.handlers[MethodBrowserConnect] = server.handleBrowserConnect
	server.handlers[MethodBrowserOpen] = server.handleBrowserOpen
	server.handlers[MethodBrowserPages] = server.handleBrowserPages
	server.handlers[MethodBrowserSwitch] = server.handleBrowserSwitch
	server.handlers[MethodBrowserClosePage] = server.handleBrowserClosePage
	server.handlers[MethodBrowserClick] = server.handleBrowserClick
	server.handlers[MethodBrowserInput] = server.handleBrowserInput
	server.handlers[MethodBrowserWait] = server.handleBrowserWait
	server.handlers[MethodBrowserAccessibility] = server.handleBrowserAccessibility
	server.handlers[MethodBrowserChallenge] = server.handleBrowserChallenge
	server.handlers[MethodBrowserScreenshot] = server.handleBrowserScreenshot
	server.handlers[MethodBrowserPDF] = server.handleBrowserPDF
	server.handlers[MethodBrowserDownload] = server.handleBrowserDownload
	server.handlers[MethodBrowserMetadata] = server.handleBrowserMetadata
}
