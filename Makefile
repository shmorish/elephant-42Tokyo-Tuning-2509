ssh-vm:
	ssh -i ftt2508-app-elephant_key.pem azureuser@20.222.176.219

run:
	bash run.sh

deploy:
	git pull origin main
	make run

restart-backend:
	cd webapp && docker-compose -f docker-compose.local.yml up -d --build backend

restore-vm:
	git clone https://github.com/DreamArts/42Tokyo-Tuning-2509.git && cd 42Tokyo-Tuning-2509 && sed -i '8,11d' entry.sh && echo 'https://github.com/shmorish/elephant-42Tokyo-Tuning-2509.git' | bash entry.sh

nginx:
	docker exec -it tuning-nginx bash

mysql:
	docker exec -it tuning-mysql mysql -u root -p

alp:
	docker exec -it tuning-nginx bash alp.sh
