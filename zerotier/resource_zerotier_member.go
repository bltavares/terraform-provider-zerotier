package zerotier

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
)

func resourceZeroTierMember() *schema.Resource {
	return &schema.Resource{
		Create: resourceMemberCreate,
		Read:   resourceMemberRead,
		Update: resourceMemberUpdate,
		Delete: resourceMemberDelete,
		Exists: resourceMemberExists,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"network_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"node_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Managed by Terraform",
			},
			"hidden": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"offline_notify_delay": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  0,
			},
			"authorized": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
			"allow_ethernet_bridging": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"no_auto_assign_ips": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"ip_assignments": {
				Type:        schema.TypeSet,
				Description: "List of IP routed and assigned by ZeroTier controller assignment pool. Does not include RFC4193 nor 6PLANE addresses, only those from assignment pool or manually provided.",
				Optional:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"ipv4_assignments": {
				Type:        schema.TypeSet,
				Description: "Computed list of IPv4 assigned by ZeroTier controller assignment pool. Does not include RFC4193 nor 6PLANE addresses, only those from assignment pool or manually provided.",
				Computed:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"ipv6_assignments": {
				Type:        schema.TypeSet,
				Description: "Computed list of IPv6 assigned by ZeroTier controller assignment pool. Does not include RFC4193 nor 6PLANE addresses, only those from assignment pool or manually provided.",
				Computed:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"rfc4193_address": {
				Type:        schema.TypeString,
				Description: "Computed RFC4193 (IPv6 /128) address. Always calculated and only actually assigned on the member if RFC4193 is configured on the network.",
				Computed:    true,
			},
			"zt6plane_address": {
				Type:        schema.TypeString,
				Description: "Computed 6PLANE (IPv6 /60) address. Always calculated and only actually assigned on the member if 6PLANE is configured on the network.",
				Computed:    true,
			},
			"capabilities": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeInt,
				},
			},
			"tags": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeInt,
				},
			},
		},
	}
}

func resourceMemberCreate(d *schema.ResourceData, m interface{}) error {
	client := m.(*ZeroTierClient)
	stored, err := memberFromResourceData(d)
	if err != nil {
		return err
	}
	created, err := client.CreateMember(stored)
	if err != nil {
		return err
	}
	d.SetId(created.Id)
	setTags(d, created)
	return nil
}

func resourceMemberUpdate(d *schema.ResourceData, m interface{}) error {
	client := m.(*ZeroTierClient)
	stored, err := memberFromResourceData(d)
	if err != nil {
		return err
	}
	updated, err := client.UpdateMember(stored)
	if err != nil {
		return fmt.Errorf("unable to update member using ZeroTier API: %s", err)
	}
	setTags(d, updated)
	return nil
}

func setTags(d *schema.ResourceData, member *Member) {
	rawTags := map[string]int{}
	for _, tuple := range member.Config.Tags {
		key := fmt.Sprintf("%d", tuple[0])
		val := tuple[1]
		rawTags[key] = val
	}
}

func resourceMemberDelete(d *schema.ResourceData, m interface{}) error {
	client := m.(*ZeroTierClient)
	member, err := memberFromResourceData(d)
	if err != nil {
		return err
	}
	err = client.DeleteMember(member)
	return err
}

func memberFromResourceData(d *schema.ResourceData) (*Member, error) {
	tags := d.Get("tags").(map[string]interface{})
	tagTuples := [][]int{}
	for key, val := range tags {
		i, err := strconv.Atoi(key)
		if err != nil {
			break
		}
		tagTuples = append(tagTuples, []int{i, val.(int)})
	}
	capsRaw := d.Get("capabilities").(*schema.Set).List()
	caps := make([]int, len(capsRaw))
	for i := range capsRaw {
		caps[i] = capsRaw[i].(int)
	}
	ipsRaw := d.Get("ip_assignments").(*schema.Set).List()
	ips := make([]string, len(ipsRaw))
	for i := range ipsRaw {
		ips[i] = ipsRaw[i].(string)
	}
	n := &Member{
		Id:                 d.Id(),
		NetworkId:          d.Get("network_id").(string),
		NodeId:             d.Get("node_id").(string),
		Hidden:             d.Get("hidden").(bool),
		OfflineNotifyDelay: d.Get("offline_notify_delay").(int),
		Name:               d.Get("name").(string),
		Description:        d.Get("description").(string),
		Config: &MemberConfig{
			Authorized:      d.Get("authorized").(bool),
			ActiveBridge:    d.Get("allow_ethernet_bridging").(bool),
			NoAutoAssignIps: d.Get("no_auto_assign_ips").(bool),
			Capabilities:    caps,
			Tags:            tagTuples,
			IpAssignments:   ips,
		},
	}
	return n, nil
}

// Extracts the Network ID and Node ID from the resource definition, or from the id during import
//
// When importing a resource, both the network id and node id writen on the definition will be ignored
// and we could retrieve the network id and node id from parts of the id
// which is formated as <network-id>-<node-id> on zerotier
func resourceNetworkAndNodeIdentifiers(d *schema.ResourceData) (string, string) {
	nwid := d.Get("network_id").(string)
	nodeID := d.Get("node_id").(string)

	if nwid == "" && nodeID == "" {
		parts := strings.Split(d.Id(), "-")
		nwid, nodeID = parts[0], parts[1]
	}
	return nwid, nodeID
}

// Receive a string and format every 4th element with a ":"
func buildIPV6(data string) (result string) {
	s := strings.SplitAfter(data, "")
	end := len(s) - 1
	result = ""
	for i, s := range s {
		result += s
		if (i+1)%4 == 0 && i != end {
			result += ":"
		}
	}
	return
}

// Calculate 6PLANE address for the member
func sixPlaneAddress(d *schema.ResourceData) string {
	nwid, nodeID := resourceNetworkAndNodeIdentifiers(d)
	return buildIPV6("fd" + nwid + "9993" + nodeID)
}

// Calculate RFC4193 address for the member
func rfc4193Address(d *schema.ResourceData) string {
	nwid, nodeID := resourceNetworkAndNodeIdentifiers(d)
	nwidInt, _ := strconv.ParseUint(nwid, 16, 64)
	networkMask := uint32((nwidInt >> 32) ^ nwidInt)
	networkPrefix := strconv.FormatUint(uint64(networkMask), 16)
	return buildIPV6("fc" + networkPrefix + nodeID + "000000000001")
}

// Split the list of assigned IPs into IPv6 and IPv4 lists
// Does not include 6PLANE or RFC4193, only those from the assignment pool
func assingnedIpsGrouping(ipAssignments []string) (ipv4s []string, ipv6s []string) {
	for _, element := range ipAssignments {
		if strings.Contains(element, ":") {
			ipv6s = append(ipv6s, element)
		} else {
			ipv4s = append(ipv4s, element)
		}
	}
	return
}

func resourceMemberRead(d *schema.ResourceData, m interface{}) error {
	client := m.(*ZeroTierClient)

	// Attempt to read from an upstream API
	nwid, nodeId := resourceNetworkAndNodeIdentifiers(d)
	member, err := client.GetMember(nwid, nodeId)

	// If the resource does not exist, inform Terraform. We want to immediately
	// return here to prevent further processing.
	if err != nil {
		return fmt.Errorf("unable to read network from API: %s", err)
	}
	if member == nil {
		d.SetId("")
		return nil
	}

	ipv4Assignments, ipv6Assignments := assingnedIpsGrouping(member.Config.IpAssignments)

	d.SetId(member.Id)
	d.Set("name", member.Name)
	d.Set("description", member.Description)
	d.Set("node_id", nodeId)
	d.Set("network_id", nwid)
	d.Set("hidden", member.Hidden)
	d.Set("offline_notify_delay", member.OfflineNotifyDelay)
	d.Set("authorized", member.Config.Authorized)
	d.Set("allow_ethernet_bridging", member.Config.ActiveBridge)
	d.Set("no_auto_assign_ips", member.Config.NoAutoAssignIps)
	d.Set("ip_assignments", member.Config.IpAssignments)
	d.Set("ipv4_assignments", ipv4Assignments)
	d.Set("ipv6_assignments", ipv6Assignments)
	d.Set("rfc4193_address", rfc4193Address(d))
	d.Set("zt6plane_address", sixPlaneAddress(d))
	d.Set("capabilities", member.Config.Capabilities)
	setTags(d, member)

	return nil
}

func resourceMemberExists(d *schema.ResourceData, m interface{}) (b bool, e error) {
	client := m.(*ZeroTierClient)
	nwid, nodeId := resourceNetworkAndNodeIdentifiers(d)
	exists, err := client.CheckMemberExists(nwid, nodeId)
	if err != nil {
		return exists, err
	}

	if !exists {
		d.SetId("")
	}
	return exists, nil
}
