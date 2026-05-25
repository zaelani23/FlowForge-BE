docker:
	docker build -t workflow-engine:latest .
	docker tag workflow-engine:latest ghcr.io/zaelani23/flowforge-be:latest
	docker push ghcr.io/zaelani23/flowforge-be:latest