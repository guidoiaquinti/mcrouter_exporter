package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
)

const (
	namespace = "mcrouter"
)

// Exporter collects metrics from a mcrouter server.
// For the latest list of exposed metrics see:
//   - https://github.com/facebook/mcrouter/blob/master/mcrouter/stat_list.h
//   - https://github.com/facebook/mcrouter/blob/master/mcrouter/lib/network/gen/MemcacheRouterStats.h
//
type Exporter struct {
	server  string
	timeout time.Duration

	up *prometheus.Desc

	//
	// Basic stats/information
	//
	version     *prometheus.Desc // version
	commandArgs *prometheus.Desc // commandargs
	pid         *prometheus.Desc // pid
	parentPid   *prometheus.Desc // parent_pid
	time        *prometheus.Desc // time

	// how long ago we started up libmcrouter instance
	uptime *prometheus.Desc // uptime

	//
	// Standalone mcrouter stats (not applicable to libmcrouter)
	//
	//     num_client_connections
	//
	// TODO

	//
	// Stats related to connections to servers
	//
	//     num_servers
	//     num_servers_new
	//     num_servers_up
	//     num_servers_down
	//     num_servers_closed
	servers *prometheus.Desc

	// same as above, but for ssl.
	//     num_ssl_servers
	//     num_ssl_servers_new
	//     num_ssl_servers_up
	//     num_ssl_servers_down
	//     num_ssl_servers_closed
	serversSSL *prometheus.Desc

	// number of servers marked-down/suspect
	//     num_suspect_servers
	serversSuspect *prometheus.Desc

	// Running total of connection opens/closes
	//     num_connections_opened
	//     num_connections_closed
	connections *prometheus.Desc

	// Running total of ssl connection opens/closes.
	//     num_ssl_connections_opened
	//     num_ssl_connections_closed
	//
	// Running total of successful SSL connection attempts/successes
	//     num_ssl_resumption_attempts
	//     num_ssl_resumption_successes
	connectionsSSL *prometheus.Desc

	// time between closing an inactive connection and opening it again.
	//     inactive_connection_closed_interval_sec
	inactiveConnectionClosedInterval *prometheus.Desc

	// Information about connect retries
	//     num_connect_success_after_retrying
	//     num_connect_retries
	//
	connect *prometheus.Desc

	// Connections closed due to retransmits
	//     retrans_closed_connections
	//
	connectClosedRetrans *prometheus.Desc

	// // OutstandingLimitRoute (OLR) queue-related stats, broken down by request type
	// // (get-like and update-like).
	// //
	// // Number of requests/second that couldn't be processed immediately in OLR
	// //     outstanding_route_get_reqs_queued
	// //     outstanding_route_update_reqs_queued
	// //
	// // Average number of requests waiting in OLR at any given time
	// //     outstanding_route_get_avg_queue_size
	// //     outstanding_route_update_avg_queue_size
	// //
	// // Average time a request had to wait in OLR
	// //     outstanding_route_get_avg_wait_time_sec
	// //     outstanding_route_update_avg_wait_time_sec
	// //
	// // OutstandingLimitRoute queue-related helper stats
	// //     outstanding_route_get_reqs_queued_helper
	// //     outstanding_route_update_reqs_queued_helper
	// //     outstanding_route_get_wait_time_sum_us
	// //     outstanding_route_update_wait_time_sum_us
	// outstandingRoute *prometheus.Desc

	// Retransmission per byte helper stats
	//     retrans_num_total
	//     retrans_per_kbyte_sum
	//     retrans_per_kbyte_max
	//     retrans_per_kbyte_avg
	retrans *prometheus.Desc

	//
	// Stats about the process
	//
	//     rusage_system
	//     rusage_user
	//     ps_rss
	//     ps_num_minor_faults
	//     ps_num_major_faults
	//     ps_user_time_sec
	//     ps_system_time_sec
	//     ps_vsize
	process *prometheus.Desc

	//
	// Stats about fibers
	//
	//     fibers_allocated
	//     fibers_pool_size
	//     fibers_stack_high_watermark
	fibers *prometheus.Desc

	//
	// Stats about routing
	//
	// number of requests that were spooled to disk
	//     asynclog_requests
	asynclogRequests *prometheus.Desc

	// Proxy requests that are currently being routed.
	//     proxy_reqs_processing
	//
	// Proxy requests queued up and not routed yet
	//     proxy_reqs_waiting
	//     proxy_request_num_outstanding
	proxyReqs *prometheus.Desc

	//     client_queue_notify_period
	// 	   client_queue_notifications
	clientQueue *prometheus.Desc

	//     dev_null_requests
	devNullRequests    *prometheus.Desc

	//     rate_limited_log_count
	log *prometheus.Desc

	//     load_balancer_load_reset_count
	loadBalancer *prometheus.Desc

	//     request_sent_count
	//     request_error_count
	//     request_success_count
	//     request_replied_count
	//     request_sent
	//     request_error
	//     request_success
	//     request_replied
	requests *prometheus.Desc

	//     failover_all
	//     failover_conditional
	//     failover_all_failed
	//     failover_rate_limited
	//     failover_inorder_policy
	//     failover_inorder_policy_failed
	//     failover_least_failures_policy
	//     failover_least_failures_policy_failed
	//     failover_custom_policy
	//     failover_custom_policy_failed
	//     failover_custom_limit_reached
	//     failover_custom_master_region
	//     failover_custom_master_region_skipped
	failover *prometheus.Desc

	//      result_error_count
	//      result_error_all_count
	//      result_connect_error_count
	//      result_connect_error_all_count
	//      result_connect_timeout_count
	//      result_connect_timeout_all_count
	//      result_data_timeout_count
	//      result_data_timeout_all_count
	//      result_busy_count
	//      result_busy_all_count
	//      result_tko_count
	//      result_tko_all_count
	//      result_client_error_count
	//      result_client_error_all_count
	//      result_local_error_count
	//      result_local_error_all_count
	//      result_remote_error_count
	//      result_remote_error_all_count
	//
	//      result_error
	//      result_error_all
	//      result_connect_error
	//      result_connect_error_all
	//      result_connect_timeout
	//      result_connect_timeout_all
	//      result_data_timeout
	//      result_data_timeout_all
	//      result_busy
	//      result_busy_all
	//      result_tko
	//      result_tko_all
	//      result_client_error
	//      result_client_error_all
	//      result_local_error
	//      result_local_error_all
	//      result_remote_error
	//      result_remote_error_all
	//
	//      final_result_error
	result *prometheus.Desc

	// Stats about RPC (AsyncMcClient)
	//
	//     destination_batches_sum
	//     destination_requests_sum
	//
	// count the dirty requests (i.e. requests that needed reserialization)
	//     destination_reqs_dirty_buffer_sum
	//     destination_reqs_total_sum
	//
	// Total reqs in AsyncMcClient yet to be sent to memcache.
	//     destination_pending_reqs
	//
	// Total reqs waiting for reply from server.
	//     destination_inflight_reqs
	//     destination_batch_size
	//     destination_reqs_dirty_buffer_ratio
	//
	//     destination_max_pending_reqs
	//     destination_max_inflight_reqs
	//
	// stats about sending lease-set to where the lease-get came from
	//     redirected_lease_set_count
	//
	// duration of the rpc call
	//     duration_us
	//
	// Duration microseconds, broken down by request type (get-like and update-like)
	//     duration_get_us
	//     duration_update_us
	//
	// This is the total number of times we called write to a socket.
	//     num_socket_writes
	//
	// This is the number of times we couldn't complete a full write.
	// Note: this can be larger than num_socket_writes. E.g. if we called write
	// on a huge buffer, we would partially write it many times.
	//    num_socket_partial_writes
	//
	//    replies_compressed
	//    replies_not_compressed
	//    reply_traffic_before_compression
	//    reply_traffic_after_compression
	reply *prometheus.Desc

	// Stats about configuration
	//
	//    config_age
	//    config_last_attempt
	//    config_last_success
	//    config_failures
	//    configs_from_disk
	config *prometheus.Desc

	// Command
	//
	//    cmd_add_count
	//    cmd_append_count
	//    cmd_cas_count
	//    cmd_decr_count
	//    cmd_delete_count
	//    cmd_flushall_count
	//    cmd_flushre_count
	//    cmd_gat_count
	//    cmd_gats_count
	//    cmd_get_count
	//    cmd_gets_count
	//    cmd_incr_count
	//    cmd_lease_get_count
	//    cmd_lease_set_count
	//    cmd_metaget_count
	//    cmd_prepend_count
	//    cmd_replace_count
	//    cmd_set_count
	//    cmd_touch_count
	//    cmd_add
	//    cmd_append
	//    cmd_cas
	//    cmd_decr
	//    cmd_delete
	//    cmd_flushall
	//    cmd_flushre
	//    cmd_gat
	//    cmd_gats
	//    cmd_get
	//    cmd_gets
	//    cmd_incr
	//    cmd_lease_get
	//    cmd_lease_set
	//    cmd_metaget
	//    cmd_prepend
	//    cmd_replace
	//    cmd_set
	//    cmd_touch
	//    cmd_add_out
	//    cmd_append_out
	//    cmd_cas_out
	//    cmd_decr_out
	//    cmd_delete_out
	//    cmd_flushall_out
	//    cmd_flushre_out
	//    cmd_gat_out
	//    cmd_gats_out
	//    cmd_get_out
	//    cmd_gets_out
	//    cmd_incr_out
	//    cmd_lease_get_out
	//    cmd_lease_set_out
	//    cmd_metaget_out
	//    cmd_prepend_out
	//    cmd_replace_out
	//    cmd_set_out
	//    cmd_touch_out
	//    cmd_add_out_all
	//    cmd_append_out_all
	//    cmd_cas_out_all
	//    cmd_decr_out_all
	//    cmd_delete_out_all
	//    cmd_flushall_out_all
	//    cmd_flushre_out_all
	//    cmd_gat_out_all
	//    cmd_gats_out_all
	//    cmd_get_out_all
	//    cmd_gets_out_all
	//    cmd_incr_out_all
	//    cmd_lease_get_out_all
	//    cmd_lease_set_out_all
	//    cmd_metaget_out_all
	//    cmd_prepend_out_all
	//    cmd_replace_out_all
	//    cmd_set_out_all
	//    cmd_touch_out_all
	command *prometheus.Desc

}

// NewExporter returns an initialized exporter.
func NewExporter(server string, timeout time.Duration) *Exporter {
	return &Exporter{
		server:  server,
		timeout: timeout,
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Whether the collector succeeded.",
			nil,
			nil,
		),
		version: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "version"),
			"Version of the mcrouter binary.",
			[]string{"version"},
			nil,
		),
		commandArgs: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "commandargs"),
			"Command args used.",
			[]string{"commandargs"},
			nil,
		),
		pid: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "pid"),
			"PID of the process",
			nil,
			nil,
		),
		parentPid: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "parent_pid"),
			"Parent PID of the process",
			nil,
			nil,
		),
		time: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "time"),
			"The UNIX epoch of the mcrouter daemon start.",
			nil,
			nil,
		),
		uptime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "uptime"),
			"Uptime of the mcrouter daemon.",
			nil,
			nil,
		),
		servers: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "servers"),
			"Stats related to connections to servers.",
			[]string{"state"},
			nil,
		),
		serversSSL: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "servers_ssl"),
			"Stats related to connections to servers via SSL.",
			[]string{"state"},
			nil,
		),
		serversSuspect: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "servers_suspect"),
			"Number of servers marked-down/suspect.",
			nil,
			nil,
		),
		connections: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connections"),
			"Running total of connection opens/closes.",
			[]string{"state"},
			nil,
		),
		connectionsSSL: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connections_ssl"),
			"Running total of SSL connection opens/closes & successful SSL connection attempts/successes",
			[]string{"state"},
			nil,
		),
		inactiveConnectionClosedInterval: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "inactive_connection_closed_interval"),
			"Time between closing an inactive connection and opening it again (in seconds).",
			nil,
			nil,
		),
		connect: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connect"),
			"Connect retries stats",
			[]string{"type"},
			nil,
		),
		connectClosedRetrans: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connections_closed_retrans"),
			"Connection closed due to retransmits.",
			nil,
			nil,
		),

		// TODO: OLR

		retrans: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "retrans"),
			"Retransmission per byte helper stats.",
			[]string{"type"},
			nil,
		),
		process: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "process"),
			"Process stats.",
			[]string{"type"},
			nil,
		),
		fibers: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "fibers"),
			"Fibers stats.",
			[]string{"type"},
			nil,
		),
		asynclogRequests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "asynclog_requests"),
			"Number of failed deletes written to spool file.",
			nil,
			nil,
		),
		proxyReqs: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "proxy_reqs"),
			"Proxy request stats.",
			[]string{"type"},
			nil,
		),
		clientQueue: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "client_queue"),
			"Client queue stats.",
			[]string{"type"},
			nil,
		),
		devNullRequests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "dev_null_requests"),
			"Number of requests sent to DevNullRoute.",
			nil,
			nil,
		),
		log: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "log"),
			"Log stats.",
			[]string{"type"},
			nil,
		),
		loadBalancer: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "load_balancer"),
			"Load balancer stats.",
			[]string{"type"},
			nil,
		),
		requests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "requests"),
			"Requests stats.",
			[]string{"type"},
			nil,
		),
		failover: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "failover"),
			"Requests stats.",
			[]string{"type"},
			nil,
		),
		result: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "result"),
			"Result stats.",
			[]string{"type"},
			nil,
		),

		// TODO: Stats about RPC (AsyncMcClient)


		config: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "config"),
			"Config stats.",
			[]string{"type"},
			nil,
		),

		command: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "command"),
			"Command stats.",
			[]string{"type"},
			nil,
		),
	}
}

// Describe describes all the metrics exported by the mcrouter exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up
	ch <- e.version
	ch <- e.commandArgs
	ch <- e.pid
	ch <- e.parentPid
	ch <- e.time
	ch <- e.uptime
	ch <- e.servers
	ch <- e.serversSSL
	ch <- e.serversSuspect
	ch <- e.connections
	ch <- e.connectionsSSL
	ch <- e.inactiveConnectionClosedInterval
	ch <- e.connect
	ch <- e.connectClosedRetrans
	// TODO: OLR
	ch <- e.retrans
	ch <- e.process
	ch <- e.fibers
	ch <- e.asynclogRequests
	ch <- e.proxyReqs
	ch <- e.clientQueue
	ch <- e.devNullRequests
	ch <- e.log
	ch <- e.requests
	ch <- e.failover
	ch <- e.result
	// TODO: Stats about RPC (AsyncMcClient)
	ch <- e.config
	ch <- e.command
}

// Collect fetches the statistics from the configured mcrouter server, and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	network := "tcp"
	if strings.Contains(e.server, "/") {
		network = "unix"
	}

	conn, err := net.DialTimeout(network, e.server, e.timeout)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		log.Errorf("Failed to connect to mcrouter: %s.", err)
		return
	}

	defer conn.Close()

	s, err := getStats(conn)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		log.Errorf("Failed to collect stats from mcrouter: %s", err)
		return
	}

	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 1)

	// Parse basic stats
	ch <- prometheus.MustNewConstMetric(e.version, prometheus.GaugeValue, 1, s["version"])
	ch <- prometheus.MustNewConstMetric(e.commandArgs, prometheus.GaugeValue, 1, s["commandargs"])
	ch <- prometheus.MustNewConstMetric(e.pid, prometheus.CounterValue, parse(s, "pid"))
	ch <- prometheus.MustNewConstMetric(e.parentPid, prometheus.CounterValue, parse(s, "parent_pid"))
	ch <- prometheus.MustNewConstMetric(e.time, prometheus.CounterValue, parse(s, "time"))
	ch <- prometheus.MustNewConstMetric(e.uptime, prometheus.CounterValue, parse(s, "uptime"))

	// Servers
	for _, op := range []string{"new", "up", "down", "closed"} {
		key := "num_servers_" + op
		ch <- prometheus.MustNewConstMetric(
			e.servers, prometheus.GaugeValue, parse(s, key), op)
	}

	// Servers (SSL)
	for _, op := range []string{"new", "up", "down", "closed"} {
		key := "num_ssl_servers_" + op
		ch <- prometheus.MustNewConstMetric(
			e.serversSSL, prometheus.GaugeValue, parse(s, key), op)
	}

	ch <- prometheus.MustNewConstMetric(e.serversSuspect, prometheus.CounterValue, parse(s, "num_suspect_servers"))

	// Connections
	for _, op := range []string{"opened", "closed"} {
		key := "num_connections_" + op
		ch <- prometheus.MustNewConstMetric(
			e.connections, prometheus.GaugeValue, parse(s, key), op)
	}

	// Connections (SSL)
	for _, op := range []string{"opened", "closed"} {
		key := "num_ssl_connections_" + op
		ch <- prometheus.MustNewConstMetric(
			e.connectionsSSL, prometheus.GaugeValue, parse(s, key), op)
	}
	for _, op := range []string{"resumption_attempts", "resumption_successes"} {
		key := "num_ssl_" + op
		ch <- prometheus.MustNewConstMetric(
			e.connectionsSSL, prometheus.GaugeValue, parse(s, key), op)
	}

	ch <- prometheus.MustNewConstMetric(e.inactiveConnectionClosedInterval, prometheus.CounterValue, parse(s, "inactive_connection_closed_interval_sec"))

	for _, op := range []string{"success_after_retrying", "retries"} {
		key := "num_connect_" + op
		ch <- prometheus.MustNewConstMetric(
			e.connect, prometheus.GaugeValue, parse(s, key), op)
	}

	ch <- prometheus.MustNewConstMetric(e.connectClosedRetrans, prometheus.CounterValue, parse(s, "retrans_closed_connections"))

	// TODO: OLR
	for _, op := range []string{"num_total", "per_kbyte_sum", "per_kbyte_max", "per_kbyte_avg"} {
		key := "retrans_" + op
		ch <- prometheus.MustNewConstMetric(
			e.retrans, prometheus.GaugeValue, parse(s, key), op)
	}

	// Process
	for _, op := range []string{"rss", "num_minor_faults", "num_major_faults", "user_time_sec", "system_time_sec", "vsize" } {
		key := "ps_" + op
		ch <- prometheus.MustNewConstMetric(
			e.process, prometheus.GaugeValue, parse(s, key), op)
	}
	for _, op := range []string{"system", "user"} {
		key := "rusage_" + op
		ch <- prometheus.MustNewConstMetric(
			e.process, prometheus.GaugeValue, parse(s, key), key)
	}

	// Fibers
	for _, op := range []string{"allocated", "pool_size", "stack_high_watermark" } {
		key := "fibers_" + op
		ch <- prometheus.MustNewConstMetric(
			e.fibers, prometheus.GaugeValue, parse(s, key), op)
	}

	// Routing
	ch <- prometheus.MustNewConstMetric(e.asynclogRequests, prometheus.CounterValue, parse(s, "asynclog_requests"))

	// Proxy requests
	for _, op := range []string{"processing", "waiting" } {
		key := "proxy_reqs_" + op
		ch <- prometheus.MustNewConstMetric(
			e.proxyReqs, prometheus.GaugeValue, parse(s, key), op)
	}

	ch <- prometheus.MustNewConstMetric(e.proxyReqs, prometheus.GaugeValue, parse(s, "proxy_request_num_outstanding"), "outstanding")

	ch <- prometheus.MustNewConstMetric(e.clientQueue, prometheus.GaugeValue, parse(s, "client_queue_notify_period"), "notify_period")
	ch <- prometheus.MustNewConstMetric(e.clientQueue, prometheus.GaugeValue, parse(s, "client_queue_notifications"), "notifications")

	ch <- prometheus.MustNewConstMetric(e.devNullRequests, prometheus.CounterValue, parse(s, "dev_null_requests"))

	ch <- prometheus.MustNewConstMetric(e.log, prometheus.CounterValue, parse(s, "rate_limited_log_count"), "rate_limited_count")

	ch <- prometheus.MustNewConstMetric(e.loadBalancer, prometheus.CounterValue, parse(s, "load_balancer_load_reset_count"), "load_reset_count")

	// Requests
	for _, op := range []string{"sent_count", "error_count", "success_count", "replied_count", "sent", "error", "success", "replied" } {
		key := "request_" + op
		ch <- prometheus.MustNewConstMetric(
			e.requests, prometheus.GaugeValue, parse(s, key), op)
	}

	// Failover
	for _, op := range []string{"all", "conditional", "all_failed", "rate_limited", "inorder_policy", "inorder_policy_failed", "least_failures_policy", "least_failures_policy_failed", "custom_policy", "custom_policy_failed", "failover_custom_limit_reached", "failover_custom_master_region", "failover_custom_master_region_skipped" } {
		key := "failover_" + op
		ch <- prometheus.MustNewConstMetric(
			e.requests, prometheus.GaugeValue, parse(s, key), op)
	}

	// Results
	for _, op := range []string{"error", "connect_error", "connect_timeout", "data_timeout", "busy_count", "tko", "client_error", "local_error", "remote_error" } {
		key := "result_" + op
		ch <- prometheus.MustNewConstMetric(
			e.result, prometheus.GaugeValue, parse(s, key), op)
		ch <- prometheus.MustNewConstMetric(
			e.result, prometheus.GaugeValue, parse(s, key+"_count"), op+"_count")
		ch <- prometheus.MustNewConstMetric(
			e.result, prometheus.GaugeValue, parse(s, key+"_all"), op+"_all")
		ch <- prometheus.MustNewConstMetric(
			e.result, prometheus.GaugeValue, parse(s, key+"_all_count"), op+"_all_count")
	}
	ch <- prometheus.MustNewConstMetric(e.result, prometheus.GaugeValue, parse(s, "final_result_error"), "final_result_error")

	// Config
	for _, op := range []string{"age", "last_attempt", "last_success", "failures" } {
		key := "config_" + op
		ch <- prometheus.MustNewConstMetric(
			e.config, prometheus.GaugeValue, parse(s, key), op)
	}
	ch <- prometheus.MustNewConstMetric(e.config, prometheus.GaugeValue, parse(s, "configs_from_disk"), "from_disk")

	// Commands
	for _, op := range []string{"add", "append", "cas", "decr", "delete", "flushall", "flushre", "gat", "gats", "get", "gets", "incr", "lease_get", "lease_set", "metaget", "prepend", "replace", "set", "touch" } {
		key := "cmd_" + op
		ch <- prometheus.MustNewConstMetric(
			e.command, prometheus.GaugeValue, parse(s, key), op)
		ch <- prometheus.MustNewConstMetric(
			e.command, prometheus.GaugeValue, parse(s, key+"_count"), op+"_count")
		ch <- prometheus.MustNewConstMetric(
			e.command, prometheus.GaugeValue, parse(s, key+"_out"), op+"_out")
		ch <- prometheus.MustNewConstMetric(
			e.command, prometheus.GaugeValue, parse(s, key+"_out_all"), op+"_out_all")
	}


}

// Parse a string into a 64 bit float suitable for Prometheus
func parse(stats map[string]string, key string) float64 {

	v := math.NaN()
	var err error

	if value, ok := stats[key]; ok {
		v, err = strconv.ParseFloat(value, 64)
		if err != nil {
			log.Errorf("Failed to parse %s %q: %s", key, stats[key], err)
		}
	} else {
		log.Infof("Unable to find '%s' in the mcrouter stats output.", key)
	}

	return v
}

// Get stats from mcrouter using net.Conn
func getStats(conn net.Conn) (map[string]string, error) {
	m := make(map[string]string)
	fmt.Fprintf(conn, "stats all\r\n")
	reader := bufio.NewReader(conn)

	// Iterate over the lines and extract the metric name and value(s)
	// example lines:
	// 	 [STAT version 37.0.0
	//	 [STAT commandargs --option1 value --flag2
	//	 END
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		if line == "END\r\n" {
			break
		}

		// Split the line into 3 components, anything after the metric name should
		// be considered as the metric value.
		result := strings.SplitN(line, " ", 3)
		value := strings.TrimRight(result[2], "\r\n")
		m[result[1]] = value
	}

	return m, nil
}

func main() {
	var (
		address       = flag.String("mcrouter.address", "localhost:5000", "mcrouter server TCP address (tcp4/tcp6) or UNIX socket path")
		timeout       = flag.Duration("mcrouter.timeout", time.Second, "mcrouter connect timeout.")
		showVersion   = flag.Bool("version", false, "Print version information.")
		listenAddress = flag.String("web.listen-address", ":9442", "Address to listen on for web interface and telemetry.")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	)
	flag.Parse()

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("mcrouter_exporter"))
		os.Exit(0)
	}

	log.Infoln("Starting mcrouter_exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	prometheus.MustRegister(NewExporter(*address, *timeout))
	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Mcrouter Exporter</title></head>
             <body>
             <h1>Mcrouter Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Infoln("Starting HTTP server on", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
