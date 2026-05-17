When setting up a containerized workflow that uses Docker-in-Docker (DinD), you use depends_on to ensure your application or runner container waits for the parent Docker daemon (dind) to start before trying to execute Docker commands. [1, 2, 3, 4] 
## 🛠️ Correct Implementation
The best practice is to couple depends_on with a condition: service_healthy check. Without a health check, Docker Compose will launch your application as soon as the dind container starts, but before the internal Docker daemon is actually ready to process incoming socket requests. [3, 5, 6] 
Here is an optimized compose.yaml blueprint: [7, 8] 

services:
  # The parent Docker Daemon (DinD)
  dind:
    image: docker:dind
    privileged: true
    environment:
      DOCKER_TLS_CERTDIR: "" # Disables TLS for simpler local setup (use with caution)
    healthcheck:
      test: ["CMD", "docker", "info"]
      interval: 5s
      timeout: 5s
      retries: 3

  # The worker application/runner that needs Docker access
  app:
    image: docker:latest
    environment:
      DOCKER_HOST: tcp://dind:2375
    depends_on:
      dind:
        condition: service_healthy # Ensures dind is fully ready, not just starting
    command: sh -c "while true; do docker ps; sleep 10; done"

## ⚠️ Common Pitfalls to Avoid

* Missing privileged: true on the DinD container: The dind service must run with privileged flags to properly isolate filesystems and nested containers. [9, 10, 11, 12] 
* Incorrect DOCKER_HOST configuration: Your application container cannot communicate with the dind daemon via /var/run/docker.sock unless you are explicitly volume-sharing it. For the setup above, you must route traffic via TCP (tcp://dind:2375) using Compose's automatic internal DNS. [13, 14, 15] 
* Omitting the Health Check: Simply typing depends_on: [dind] (short syntax) frequently causes application failures because the application container will execute its startup script before the dind internal daemon finishes its initial setup boot sequence. [3, 5, 16, 17] 

Would you like help adapting this setup for a specific CI/CD platform like GitLab Runner or GitHub Actions, or do you need help enabling TLS encryption between your containers? [13, 18, 19, 20] 

[1] [https://docs.docker.com](https://docs.docker.com/compose/how-tos/startup-order/)
[2] [https://www.docker.com](https://www.docker.com/resources/docker-in-docker-containerized-ci-workflows-dockercon-2023/)
[3] [https://www.youtube.com](https://www.youtube.com/watch?v=Z-8-yXLFF-U)
[4] [https://www.warp.dev](https://www.warp.dev/terminus/docker-compose-depends-on)
[5] [https://www.youtube.com](https://www.youtube.com/watch?v=pCY6khpKqM4)
[6] [https://github.com](https://github.com/tulios/kafkajs/issues/803)
[7] [https://docs.docker.com](https://docs.docker.com/reference/compose-file/services/)
[8] [https://stackoverflow.com](https://stackoverflow.com/questions/76992733/conditional-depends-on-in-compose-yml)
[9] [https://github.com](https://github.com/deviantony/dind/blob/master/swarm-mode/docker-compose.yml)
[10] [https://www.baeldung.com](https://www.baeldung.com/ops/docker-in-docker)
[11] [https://tinkalshakya.medium.com](https://tinkalshakya.medium.com/run-docker-containers-in-docker-5f15d4e88e6d)
[12] [https://www.docker.com](https://www.docker.com/blog/testcontainers-cloud-vs-docker-in-docker-for-testing-scenarios/)
[13] [https://forum.gitlab.com](https://forum.gitlab.com/t/example-gitlab-runner-docker-compose-configuration/67344)
[14] [https://github.com](https://github.com/docker-library/docker/issues/303)
[15] [https://forums.docker.com](https://forums.docker.com/t/mounting-using-var-run-docker-sock-in-a-container-not-running-as-root/34390)
[16] [https://codingcops.com](https://codingcops.com/docker-containers/)
[17] [https://mihirpopat.medium.com](https://mihirpopat.medium.com/the-hidden-power-of-depends-on-in-terraform-ensuring-predictable-infrastructure-deployments-88fbd06db2d9)
[18] [https://beaglesecurity.com](https://beaglesecurity.com/blog/vulnerability/misconfigured-docker-on-default-port.html)
[19] [https://docs.docker.com](https://docs.docker.com/guides/angular/)
[20] [https://www.youtube.com](https://www.youtube.com/watch?v=XxQptki98Jc)
