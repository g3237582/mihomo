#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
	uint16_t family;
	uint16_t port;
	uint8_t address[16];
} DataPlaneSocketAddr;

typedef struct {
	uint64_t operation_id;
	uint16_t operation_kind;
	uint16_t status;
} DataPlaneCompletion;

static uint64_t next_op = 1;
static uint64_t last_op = 0;
static int pending = 0;
static DataPlaneSocketAddr last_peer;
static char last_err[256];

static void set_err(const char *msg) {
	memset(last_err, 0, sizeof(last_err));
	if (msg != NULL) {
		strncpy(last_err, msg, sizeof(last_err) - 1);
	}
}

int run_network_instance(const char *cfg_str) {
	(void)cfg_str;
	return 0;
}

int retain_network_instance(const char **inst_names, size_t length) {
	(void)inst_names;
	(void)length;
	return 0;
}

int delete_network_instance(const char **inst_names, size_t length) {
	(void)inst_names;
	(void)length;
	return 0;
}

void get_error_msg(const char **out) {
	if (out == NULL) {
		return;
	}
	if (last_err[0] == '\0') {
		*out = NULL;
		return;
	}
	char *copy = malloc(strlen(last_err) + 1);
	if (copy == NULL) {
		*out = NULL;
		return;
	}
	memcpy(copy, last_err, strlen(last_err) + 1);
	*out = copy;
}

void free_string(const char *s) {
	free((void *)s);
}

int data_plane_session_open(const char *inst_name, uint64_t *out_session) {
	if (out_session == NULL || inst_name == NULL || inst_name[0] == '\0') {
		set_err("invalid session_open arguments");
		return -1;
	}
	*out_session = 1;
	return 0;
}

int data_plane_session_close(uint64_t session) {
	(void)session;
	return 0;
}

static uint64_t submit_op(void) {
	last_op = next_op++;
	pending = 1;
	return last_op;
}

int data_plane_tcp_connect_submit(uint64_t session, DataPlaneSocketAddr peer_addr,
				  uint64_t timeout_ms, uint64_t *out_operation) {
	(void)session;
	(void)timeout_ms;
	if (out_operation == NULL) {
		set_err("out_operation is null");
		return -1;
	}
	last_peer = peer_addr;
	*out_operation = submit_op();
	return 0;
}

int data_plane_tcp_connect_result_take(uint64_t session, uint64_t operation,
				       uint64_t *out_stream,
				       DataPlaneSocketAddr *out_local,
				       DataPlaneSocketAddr *out_peer) {
	(void)session;
	(void)operation;
	if (out_stream == NULL || out_local == NULL || out_peer == NULL) {
		set_err("tcp connect result pointer is null");
		return -1;
	}
	*out_stream = 42;
	memset(out_local, 0, sizeof(*out_local));
	out_local->family = 4;
	out_local->port = 1234;
	out_local->address[0] = 10;
	out_local->address[3] = 1;
	*out_peer = last_peer;
	return 0;
}

int data_plane_tcp_read_submit(uint64_t session, uint64_t stream, uint32_t max_len,
			       uint64_t *out_operation) {
	(void)session;
	(void)stream;
	(void)max_len;
	if (out_operation == NULL) {
		set_err("out_operation is null");
		return -1;
	}
	*out_operation = submit_op();
	return 0;
}

int data_plane_tcp_read_result_take(uint64_t session, uint64_t operation, uint8_t *data,
				    uint32_t capacity, uint32_t *out_len, bool *out_eof) {
	(void)session;
	(void)operation;
	if (out_len == NULL || out_eof == NULL) {
		set_err("tcp read result pointer is null");
		return -1;
	}
	const char *payload = "hello";
	uint32_t n = (uint32_t)strlen(payload);
	if (n > capacity) {
		n = capacity;
	}
	if (data != NULL && n > 0) {
		memcpy(data, payload, n);
	}
	*out_len = n;
	*out_eof = false;
	return 0;
}

int data_plane_tcp_write_submit(uint64_t session, uint64_t stream, const uint8_t *data,
				uint32_t len, uint64_t *out_operation) {
	(void)session;
	(void)stream;
	(void)data;
	(void)len;
	if (out_operation == NULL) {
		set_err("out_operation is null");
		return -1;
	}
	*out_operation = submit_op();
	return 0;
}

int data_plane_tcp_write_result_take(uint64_t session, uint64_t operation, uint32_t *out_len) {
	(void)session;
	(void)operation;
	if (out_len == NULL) {
		set_err("out_len is null");
		return -1;
	}
	*out_len = 5;
	return 0;
}

int data_plane_udp_bind_submit(uint64_t session, uint16_t local_port, uint64_t timeout_ms,
			       uint64_t *out_operation) {
	(void)session;
	(void)local_port;
	(void)timeout_ms;
	if (out_operation == NULL) {
		set_err("out_operation is null");
		return -1;
	}
	*out_operation = submit_op();
	return 0;
}

int data_plane_udp_bind_result_take(uint64_t session, uint64_t operation, uint64_t *out_socket,
				    DataPlaneSocketAddr *out_local) {
	(void)session;
	(void)operation;
	if (out_socket == NULL || out_local == NULL) {
		set_err("udp bind result pointer is null");
		return -1;
	}
	*out_socket = 7;
	memset(out_local, 0, sizeof(*out_local));
	out_local->family = 4;
	out_local->port = 9000;
	out_local->address[0] = 10;
	out_local->address[3] = 1;
	return 0;
}

int data_plane_udp_send_submit(uint64_t session, uint64_t socket, DataPlaneSocketAddr peer_addr,
			       const uint8_t *data, uint32_t len, uint64_t *out_operation) {
	(void)session;
	(void)socket;
	(void)peer_addr;
	(void)data;
	(void)len;
	if (out_operation == NULL) {
		set_err("out_operation is null");
		return -1;
	}
	*out_operation = submit_op();
	return 0;
}

int data_plane_udp_send_result_take(uint64_t session, uint64_t operation, uint32_t *out_len) {
	(void)session;
	(void)operation;
	if (out_len == NULL) {
		set_err("out_len is null");
		return -1;
	}
	*out_len = 4;
	return 0;
}

int data_plane_udp_receive_submit(uint64_t session, uint64_t socket, uint32_t max_len,
				  uint64_t *out_operation) {
	(void)session;
	(void)socket;
	(void)max_len;
	if (out_operation == NULL) {
		set_err("out_operation is null");
		return -1;
	}
	*out_operation = submit_op();
	return 0;
}

int data_plane_udp_receive_result_take(uint64_t session, uint64_t operation, uint8_t *data,
				       uint32_t capacity, uint32_t *out_len,
				       DataPlaneSocketAddr *out_peer, bool *out_truncated) {
	(void)session;
	(void)operation;
	if (out_len == NULL || out_peer == NULL || out_truncated == NULL) {
		set_err("udp receive result pointer is null");
		return -1;
	}
	const char *payload = "pong";
	uint32_t n = (uint32_t)strlen(payload);
	if (n > capacity) {
		n = capacity;
	}
	if (data != NULL && n > 0) {
		memcpy(data, payload, n);
	}
	*out_len = n;
	memset(out_peer, 0, sizeof(*out_peer));
	out_peer->family = 4;
	out_peer->port = 53;
	out_peer->address[0] = 10;
	out_peer->address[3] = 2;
	*out_truncated = false;
	return 0;
}

int data_plane_completion_wait(uint64_t session, uint64_t timeout_ms) {
	(void)session;
	(void)timeout_ms;
	return pending ? 1 : 0;
}

int data_plane_completion_drain(uint64_t session, DataPlaneCompletion *completions,
				uint32_t capacity) {
	(void)session;
	if (capacity == 0 || completions == NULL || !pending) {
		return 0;
	}
	completions[0].operation_id = last_op;
	completions[0].operation_kind = 1;
	completions[0].status = 0;
	pending = 0;
	return 1;
}

int data_plane_operation_cancel(uint64_t session, uint64_t operation) {
	(void)session;
	(void)operation;
	return 0;
}

int data_plane_operation_free(uint64_t session, uint64_t operation) {
	(void)session;
	(void)operation;
	return 0;
}

int data_plane_resource_close(uint64_t session, uint64_t resource) {
	(void)session;
	(void)resource;
	return 0;
}

int data_plane_resource_deadline_set(uint64_t session, uint64_t resource, uint32_t direction,
				     uint64_t timeout_ms) {
	(void)session;
	(void)resource;
	(void)direction;
	(void)timeout_ms;
	return 0;
}
