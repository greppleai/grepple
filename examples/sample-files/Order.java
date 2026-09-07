package shop;

import java.util.List;

public class Order {
    public record LineItem(String sku, int quantity, double price) {}

    private final String id;
    private final List<LineItem> items;

    public Order(String id, List<LineItem> items) {
        this.id = id;
        this.items = items;
    }

    public double total() {
        return items.stream()
            .mapToDouble(item -> item.quantity() * item.price())
            .sum();
    }

    public String describe() {
        return "order " + id + " with " + items.size() + " items";
    }
}
