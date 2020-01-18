#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/init.h>
#include <linux/netfilter.h>
#include <linux/vmalloc.h>
#include <linux/netfilter_ipv6.h>
#define NETLINK_RST_HOOK 17

static struct nf_hook_ops nf_host;
static struct pernet_operations ops;
static struct netlink_kernel_cfg cfg;
static struct sock *nl_sock;
static unsigned int pid;

static unsigned int nf_tcp_hook(void *priv, struct sk_buff *skb, const struct nf_hook_state *state);

static void nl_recv_msg (struct sk_buff *skb_in) {
    struct nlmsghdr *nlh;

    nlh = (struct nlmsghdr *)skb_in->data;
    pid = nlh->nlmsg_pid;
    printk(KERN_INFO "received netlink message(%s) from PID(%d)\n", (char*)nlmsg_data(nlh), pid);
}

unsigned int nf_tcp_hook(void *priv, struct sk_buff *skb, const struct nf_hook_state *state)
{
	const struct ipv6hdr *iph = ipv6_hdr(skb);
	struct tcphdr *tcph;

	if(skb->pkt_type == PACKET_HOST) {
		return NF_DROP;
		if(iph->version != 6) {
			return NF_ACCEPT;
		}
		if (iph->nexthdr != IPPROTO_TCP) {
			return NF_ACCEPT;
		}
		tcph = tcp_hdr(skb);
		if (be16_to_cpu(tcph->source) != 8080) {
			return NF_ACCEPT;
		}
		if (!tcph->rst) {
			return NF_ACCEPT;
		}
		return NF_DROP;
	}
	return NF_ACCEPT;
}

int ns_init(struct net *net)
{
	return nf_register_net_hook(net, &nf_host);
}
void ns_exit(struct net *net)
{
	nf_unregister_net_hook(net, &nf_host);
}

static int __init nf_module_init(void)
{
	nf_host.hook = nf_tcp_hook;
	nf_host.hooknum = 0;
	nf_host.pf = PF_INET6;
	nf_host.priority = NF_INET_LOCAL_OUT;

	nf_register_net_hooks(&init_net, &nf_host, 1);

	ops.init = ns_init;
	ops.exit = ns_exit;
	register_pernet_subsys(&ops);

	cfg.input = nl_recv_msg;
	nl_sock = netlink_kernel_create(&init_net, NETLINK_RST_HOOK, &cfg);

	return 0;
}

static void __exit nf_module_exit(void)
{
	nf_unregister_net_hook(&init_net, &nf_host);
	unregister_pernet_subsys(&ops);
	netlink_kernel_release(nl_sock);
}

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Takanori Hirano");
MODULE_DESCRIPTION("rst packet hook module");

module_init(nf_module_init);
module_exit(nf_module_exit);
