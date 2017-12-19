def REGISTRY = "cargo.caicloudprivatetest.com"
def version = "${params.imageTag}"

podTemplate(
    cloud: 'dev-cluster',
    namespace: 'kube-system',
    name: 'canary-release',
    // change the label to your component name.
    label: 'canary-release',
    containers: [
        // a Jenkins agent (FKA "slave") using JNLP to establish connection.
        containerTemplate(
            name: 'jnlp',
            // alwaysPullImage: true,
            image: 'cargo.caicloudprivatetest.com/caicloud/jenkins/jnlp-slave:3.14-1-alpine',
            command: '',
            args: '${computer.jnlpmac} ${computer.name}',
        ),
        // docker in docker
        containerTemplate(
            name: 'dind',
            image: 'cargo.caicloudprivatetest.com/caicloud/docker:17.09-dind',
            ttyEnabled: true,
            command: '',
            args: '--host=unix:///home/jenkins/docker.sock',
            privileged: true,
        ),
        // golang with docker client and tools
        containerTemplate(
            name: 'golang',
            image: 'cargo.caicloudprivatetest.com/caicloud/golang-docker:1.9-17.09',
            ttyEnabled: true,
            command: '',
            args: '',
            envVars: [
                containerEnvVar(key: 'DOCKER_HOST', value: 'unix:///home/jenkins/docker.sock'),
                // Change the environment variable WORKDIR as needed.
                containerEnvVar(key: 'WORKDIR', value: '/go/src/github.com/caicloud/canary-release'),
                containerEnvVar(key: 'GO_BUILD_PLATFORMS', value: "linux/amd64"),
                containerEnvVar(key: 'PRJ_GIT_VERSION', value: "${version}"),
                containerEnvVar(key: 'REGISTRIES', value: "${REGISTRY}"+"/caicloud")
           ],
       )
   ]
) {
   // Change the node name as the podTemplate label you set.
   node('canary-release') {
       stage('Checkout') {
          checkout scm
       }
       // Change the container name as the container you use for compiling.
       container('golang') {
           ansiColor('xterm') {
               stage("Prepare Project") {
                   sh('''
                       set -e
                       mkdir -p $(dirname ${WORKDIR})
                       rm -rf ${WORKDIR}
                       cp -r $(pwd) ${WORKDIR}
                   ''')
               }

               // You can define the stage as you need.
               stage('Unit test') {
                   sh('''
                       set -e
                       cd ${WORKDIR}
                       make unittest
                   ''')
               }

               stage("Complie") {
                   sh('''
                       set -e
                       cd ${WORKDIR}
                       make build
                   ''')
               }

               stage('Build and push image') {
                    docker.withRegistry("https://${REGISTRY}", "cargo-private-admin") {
                        sh('''
                            cd ${WORKDIR}
                            make container
                        ''') 
                        if (params.publish) {
                            sh('''
                                cd ${WORKDIR}
                                make push
                            ''')
                        }
                    }
               }
           }
       }
   }
}
