CREATE TABLE job ( id INTEGER PRIMARY KEY,
 job_id TEXT, user_id TEXT, project_id TEXT, cluster_id TEXT,
 start_time INTEGER, stop_time INTEGER, duration INTEGER,
 walltime INTEGER, job_state TEXT,
 num_nodes INTEGER, node_list TEXT, has_profile INTEGER,
 mem_used REAL, flops_any REAL, mem_bw REAL, ib_bw REAL, file_bw REAL);
CREATE TABLE tag ( id INTEGER PRIMARY KEY, tag_type TEXT, tag_name TEXT);
CREATE TABLE jobtag ( job_id INTEGER, tag_id INTEGER, PRIMARY KEY (job_id, tag_id),
 FOREIGN KEY (job_id) REFERENCES job (id)  ON DELETE CASCADE ON UPDATE NO ACTION,
 FOREIGN KEY (tag_id) REFERENCES tag (id)  ON DELETE CASCADE ON UPDATE NO ACTION );
