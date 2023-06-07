ALTER TABLE job
    MODIFY `partition` VARCHAR(255),
    MODIFY array_job_id BIGINT,
    MODIFY num_hwthreads INT,
    MODIFY num_acc INT;
