package browser

func selectionSession(request PageSelection) SessionRequest {
	return SessionRequest{ProfileName: request.ProfileName, SessionName: request.SessionName}
}

func interactionSession(request InteractionRequest) SessionRequest {
	return SessionRequest{ProfileName: request.ProfileName, SessionName: request.SessionName}
}

func inputSession(request InputRequest) SessionRequest {
	return SessionRequest{ProfileName: request.ProfileName, SessionName: request.SessionName}
}

func waitSession(request WaitRequest) SessionRequest {
	return SessionRequest{ProfileName: request.ProfileName, SessionName: request.SessionName}
}

func accessibilitySession(request AccessibilityRequest) SessionRequest {
	return SessionRequest{ProfileName: request.ProfileName, SessionName: request.SessionName}
}

func downloadSession(request DownloadRequest) SessionRequest {
	return SessionRequest{ProfileName: request.ProfileName, SessionName: request.SessionName}
}

func challengeSession(request ManualChallengeRequest) SessionRequest {
	return SessionRequest{ProfileName: request.ProfileName, SessionName: request.SessionName}
}
