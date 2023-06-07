ALTER TABLE job
    MODIFY `partition` VARCHAR(255) NOT NULL,
    MODIFY array_job_id BIGINT NOT NULL,
    MODIFY num_hwthreads INT NOT NULL,
    MODIFY num_acc INT NOT NULL;
