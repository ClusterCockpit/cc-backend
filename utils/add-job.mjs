import fetch from 'node-fetch'

// Just for testing

const job = {
    jobId: 123,
    user: 'lou',
    project: 'testproj',
    cluster: 'heidi',
    partition: 'default',
    arrayJobId: 0,
    numNodes: 1,
    numHwthreads: 8,
    numAcc: 0,
    exclusive: 1,
    monitoringStatus: 1,
    smt: 1,
    jobState: 'running',
    duration: 2*60*60,
    tags: [],
    resources: [
        {
            hostname: 'heidi',
            hwthreads: [0, 1, 2, 3, 4, 5, 6, 7]
        }
    ],
    metaData: null,
    startTime: 1641427200
}

fetch('http://localhost:8080/api/jobs/start_job/', {
        method: 'POST',
        body: JSON.stringify(job),
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCJ9.eyJpc19hZG1pbiI6dHJ1ZSwiaXNfYXBpIjpmYWxzZSwic3ViIjoibG91In0.nY6dCgLSdm7zXz1xPkrb_3JnnUCgExXeXcrTlAAySs4p72VKJhmzzC1RxgkJE26l8tDYUilM-o-urzlaqK5aDA'
        }
    })
    .then(res => res.status == 200 ? res.json() : res.text())
    .then(res => console.log(res))
