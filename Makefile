ssh-vm:
	ssh -i ftt2508-app-elephant_key.pem azureuser@20.222.176.219

run:
	bash run.sh

deploy:
	git pull origin main
	make run

restart-backend:
	cd webapp && docker-compose -f docker-compose.local.yml up -d --build backend
